<script lang="ts">
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	// Sets up the iPhone Shortcuts: one key per phone, then the steps to build
	// "Snap receipt" and "Log expense" with that key and this server's address
	// filled in.
	//
	// A Shortcut can't use the login cookie, so it sends a key instead. The key
	// can list category names, log to the account it was made for, and send a
	// receipt photo — and can't read anything back.
	//
	// Only iPhones and iPads run Shortcuts, so the nav offers this page only
	// there. Listing and revoking phones lives in Settings, which works anywhere.
	import { onMount } from 'svelte';
	import { createDeviceKey, getAccounts, testDeviceKey } from '$lib/api';
	import type { Account, DeviceKey } from '$lib/types';
	import { pickRemembered } from '$lib/remember';
	import { isAppleMobile } from '$lib/device';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

	let accounts = $state<Account[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let name = $state('');
	let accountId = $state(0);
	let creating = $state(false);
	let formError = $state<string | null>(null);

	// The key just made. It exists only here: the server keeps a hash.
	let fresh = $state<{ key: DeviceKey; token: string } | null>(null);
	let test = $state<{ ok: boolean; text: string } | null>(null);
	let copied = $state<string | null>(null);
	let onAppleMobile = $state(true);

	let origin = $state('');
	let token = $derived(fresh?.token ?? 'YOUR-KEY');
	let categoriesUrl = $derived(`${origin}/api/shortcut/categories`);
	let logUrl = $derived(`${origin}/api/shortcut/log`);
	let receiptUrl = $derived(`${origin}/api/shortcut/receipt`);
	let authValue = $derived(`Bearer ${token}`);
	// A phone can only reach an address on the internet (or the tailnet).
	let localOnly = $derived(/\/\/(localhost|127\.|192\.168\.|10\.)/.test(origin));

	onMount(async () => {
		origin = location.origin;
		onAppleMobile = isAppleMobile();
		try {
			accounts = await getAccounts();
			accountId = pickRemembered('transaction.account', accounts, accounts[0]?.id ?? 0);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load your phones';
		} finally {
			loading = false;
		}
	});

	async function create(event: Event) {
		event.preventDefault();
		creating = true;
		formError = null;
		test = null;
		try {
			fresh = await createDeviceKey(name, accountId);
			name = '';
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not make a key';
		} finally {
			creating = false;
		}
	}

	async function runTest() {
		if (!fresh) return;
		test = null;
		try {
			const names = await testDeviceKey(fresh.token);
			test = { ok: true, text: `Works. The Shortcut will offer ${names.length} categories, starting with ${names.slice(0, 3).join(', ')}.` };
		} catch (e) {
			test = { ok: false, text: e instanceof Error ? e.message : 'The key did not work' };
		}
	}

	async function copy(label: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = label;
			setTimeout(() => {
				if (copied === label) copied = null;
			}, 1500);
		} catch {
			copied = null;
		}
	}

</script>

{#snippet value(label: string, text: string)}
	<span class="value">
		<code>{text}</code>
		<button type="button" class="copy" onclick={() => copy(label, text)}>
			{copied === label ? 'Copied' : 'Copy'}
		</button>
	</span>
{/snippet}

<div class="page">
	<div class="page-header">
		<div>
			<h1>iPhone Shortcut</h1>
			<p class="muted">
				Two Shortcuts, both runnable from a home-screen widget, the Action button, Back Tap or Siri
				without opening Fangorn: <strong>Snap receipt</strong> opens the camera and the photo logs
				itself; <strong>Log expense</strong> asks how much and which category.
			</p>
		</div>
	</div>

	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	{#if !onAppleMobile}
		<div class="card elsewhere">
			<p>
				The Shortcut is set up on the iPhone itself: open Fangorn there and go to
				<strong>More → iPhone Shortcut</strong>.
			</p>
			<p class="muted small">
				To see or revoke phones that already have it, go to <a href="/settings">Settings</a>.
			</p>
		</div>
	{:else if loading}
		<p class="muted">Loading…</p>
	{:else}
		<section class="card">
			<h2>1. Make a key for this phone</h2>
			<p class="muted small">
				Each phone gets its own key, so one can be switched off without touching the others. A key
				can send receipt photos, log entries to the account you pick here, and list your category
				names — it can't see balances or anything else. Phones already set up are in
				<a href="/settings">Settings</a>, where they can be revoked.
			</p>

			<form onsubmit={create}>
				<div class="form-row">
					<Field label="Phone" id="phoneName">
						<input id="phoneName" bind:value={name} placeholder="Carter's iPhone" maxlength="60" required />
					</Field>
					<Field label="Logs to" id="phoneAccount">
						<select id="phoneAccount" bind:value={accountId}>
							<AccountOptions {accounts} />
						</select>
					</Field>
				</div>
				{#if formError}
					<p class="error-text">{formError}</p>
				{/if}
				<div>
					<Button type="submit" disabled={creating || !name.trim() || !accountId}>
						{creating ? 'Making…' : 'Make a key'}
					</Button>
				</div>
			</form>

			{#if fresh}
				<div class="fresh">
					<p>
						<strong>Key for {fresh.key.name}.</strong> Copy it now — it isn't shown again. If it's lost,
						revoke it and make another.
					</p>
					{@render value('key', fresh.token)}
					<div class="test">
						<Button variant="secondary" size="sm" onclick={runTest}>Test it</Button>
						{#if test}
							<span class:pos={test.ok} class:error-text={!test.ok}>{test.text}</span>
						{/if}
					</div>
				</div>
			{/if}
		</section>

		<section class="card">
			<h2>2. Build “Snap receipt”</h2>
			{#if localOnly}
				<p class="warn-text small">
					You're on {origin}, which a phone out and about can't reach. Open this page from Fangorn's
					public address so the steps below use it.
				</p>
			{/if}
			<p class="muted small">
				On the iPhone, open <strong>Shortcuts</strong>, tap <strong>+</strong>, name it
				<strong>Snap receipt</strong>, and add{fresh ? '' : ' (make a key above first and it will be filled in here)'}:
			</p>
			<ol class="steps">
				<li><strong>Take Photo</strong> — leave <em>Show Camera Preview</em> on.</li>
				<li><strong>Resize Image</strong> — Width <em>2000</em>, Height <em>Auto</em>.</li>
				<li><strong>Convert Image</strong> — to <em>JPEG</em>.</li>
				<li>
					<strong>Get Contents of URL</strong>
					{@render value('url3', receiptUrl)}
					<em>Show More</em>: Method <em>POST</em>, header <code>Authorization</code>
					{@render value('auth3', authValue)}
					and Request Body <em>File</em> → <em>Converted Image</em>.
				</li>
				<li><strong>Show Notification</strong> — <em>Contents of URL</em></li>
			</ol>
			<p class="muted small">
				Pressing it opens the camera; after the shot it says what happened — “Posted $42.17 at Target
				on Gesa Visa.” Anything it's unsure of waits in Fangorn → Receipts.
			</p>
		</section>

		<section class="card">
			<h2>3. Build “Log expense”</h2>
			<p class="muted small">
				Another new Shortcut, named <strong>Log expense</strong> — that's also what you'll say to
				Siri:
			</p>

			<ol class="steps">
				<li>
					<strong>Ask for Input</strong> — Input: <em>Number</em>, Prompt: <em>How much?</em>
				</li>
				<li>
					<strong>Get Contents of URL</strong>
					{@render value('url1', categoriesUrl)}
					Tap <em>Show More</em> → <em>Headers</em> → <em>Add new header</em>: key
					<code>Authorization</code>, text
					{@render value('auth1', authValue)}
				</li>
				<li>
					<strong>Choose from List</strong> — List: <em>Contents of URL</em>, Prompt:
					<em>Category</em>
				</li>
				<li>
					<strong>Get Contents of URL</strong> again
					{@render value('url2', logUrl)}
					<em>Show More</em>: Method <em>POST</em>, the same <code>Authorization</code> header, and
					Request Body <em>JSON</em> with two fields:
					<ul>
						<li><code>amount</code> — Number — <em>Provided Input</em></li>
						<li><code>category</code> — Text — <em>Chosen Item</em></li>
					</ul>
				</li>
				<li><strong>Show Notification</strong> — <em>Contents of URL</em></li>
			</ol>
			<p class="muted small">
				Run it once from the Shortcuts app. The notification says what was logged — “Logged $12.50
				for Groceries on {fresh?.key.account_name ?? 'your card'}.” — or, if something's off, what to
				fix.
			</p>
		</section>

		<section class="card">
			<h2>4. Put them one tap away</h2>
			<ul class="places">
				<li>
					<strong>Home screen widget</strong> — touch and hold the home screen, tap <em>Edit</em> →
					<em>Add Widget</em> → <em>Shortcuts</em>, and pick the two-button size to have
					<em>Snap receipt</em> and <em>Log expense</em> side by side.
				</li>
				<li>
					<strong>Action button</strong> (iPhone 15 Pro and later) — Settings → Action Button →
					Shortcut → <em>Snap receipt</em>. Press it at the till.
				</li>
				<li>
					<strong>Back Tap</strong> — Settings → Accessibility → Touch → Back Tap: Double Tap →
					<em>Snap receipt</em>, Triple Tap → <em>Log expense</em>.
				</li>
				<li><strong>Siri</strong> — “Hey Siri, snap receipt” or “Hey Siri, log expense.”</li>
			</ul>
			<p class="muted small">
				<strong>Optional extras:</strong> for income, duplicate the Shortcut, add <code>?kind=income</code>
				to the first address and name it <em>Log income</em>. To add a note, insert another
				<em>Ask for Input</em> (Text) and send it as a <code>note</code> field. When sharing the
				Shortcut with family, take your key out first — each phone should use its own.
			</p>
		</section>
	{/if}
</div>

<style>
	h2 {
		font-size: 1rem;
		margin-bottom: 0.5rem;
	}

	.small {
		font-size: 0.875rem;
		line-height: 1.5;
	}

	section > p + p,
	section > p + ol,
	section > ul + p,
	section > ol + p {
		margin-top: 0.75rem;
	}

	.elsewhere {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.form-row {
		display: flex;
		gap: 1rem;
	}

	.fresh {
		margin-top: 1rem;
		padding: 0.875rem;
		border-radius: var(--radius-sm);
		background: var(--warn-soft);
		font-size: 0.875rem;
		display: flex;
		flex-direction: column;
		gap: 0.625rem;
	}

	.test {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.value {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.375rem 0;
		padding: 0.375rem 0.375rem 0.375rem 0.625rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--surface);
	}

	.value code {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
		font-size: 0.8125rem;
	}

	code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.85em;
	}

	.copy {
		flex: none;
		min-height: 36px;
		padding: 0 0.75rem;
		border: none;
		border-radius: 6px;
		background: var(--bg);
		font: inherit;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--ink);
		cursor: pointer;
	}

	.steps,
	.places {
		padding-left: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 0.875rem;
		font-size: 0.9375rem;
		line-height: 1.5;
	}

	.steps ul {
		padding-left: 1.25rem;
		margin-top: 0.25rem;
	}
</style>
