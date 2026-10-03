<script lang="ts">
	// Settings: the account this device logs to by default, the phones that can
	// log through the iPhone Shortcut, and signing out. Setting a phone up happens on the phone itself (/shortcut,
	// offered only on iPhone and iPad); seeing and revoking phones works anywhere.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStatus, deleteDeviceKey, getAccounts, getDeviceKeys, logout } from '$lib/api';
	import type { Account, DeviceKey } from '$lib/types';
	import { defaultAccount, setDefaultAccount } from '$lib/remember';
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	import { isAppleMobile } from '$lib/device';
	import Button from '$lib/components/Button.svelte';

	let keys = $state<DeviceKey[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let revoking = $state<number | null>(null);
	let loginRequired = $state(false);
	let onAppleMobile = $state(false);
	let accounts = $state<Account[]>([]);
	// 0 = no default: the log form starts on the account used last.
	let defaultId = $state(0);

	onMount(async () => {
		onAppleMobile = isAppleMobile();
		try {
			const [k, auth, a] = await Promise.all([getDeviceKeys(), authStatus(), getAccounts()]);
			keys = k;
			accounts = a;
			// A default whose account has since been closed reads as none.
			const pinned = defaultAccount();
			defaultId = a.some((x) => x.id === pinned) ? (pinned ?? 0) : 0;
			loginRequired = auth.required;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load settings';
		} finally {
			loading = false;
		}
	});

	async function revoke(key: DeviceKey) {
		revoking = key.id;
		error = null;
		try {
			await deleteDeviceKey(key.id);
			keys = keys.filter((k) => k.id !== key.id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not revoke that phone';
		} finally {
			revoking = null;
		}
	}

	function chooseDefault(id: number) {
		defaultId = id;
		setDefaultAccount(id || null);
	}

	async function signOut() {
		try {
			await logout();
		} finally {
			goto('/login');
		}
	}

	function lastUsed(iso: string | null): string {
		if (!iso) return 'never used';
		return `last used ${new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`;
	}
</script>

<div class="page">
	<div class="page-header">
		<h1>Settings</h1>
	</div>

	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	{#if accounts.length > 0}
		<section class="card">
			<h2>Default account</h2>
			<p class="muted small">
				The account a transaction you log by hand starts on. It's kept on this device, so each
				phone can have its own. A category that always goes on one account, and income, still go
				where they're set to.
			</p>
			<select
				class="default-account"
				aria-label="Default account"
				value={defaultId}
				onchange={(e) => chooseDefault(Number(e.currentTarget.value))}
			>
				<option value={0}>Last account used</option>
				<AccountOptions {accounts} />
			</select>
		</section>
	{/if}

	<section class="card">
		<div class="section-head">
			<h2>Phones</h2>
			{#if onAppleMobile}
				<a class="setup" href="/shortcut">Set up this iPhone</a>
			{/if}
		</div>
		<p class="muted small">
			Phones with the iPhone Shortcut can log expenses and send receipt photos without opening
			Fangorn. Revoking one stops its Shortcut straight away.
		</p>

		{#if loading}
			<p class="muted small">Loading…</p>
		{:else if keys.length === 0}
			<p class="empty-note small">
				No phones set up yet.
				{#if !onAppleMobile}
					To add one, open Fangorn on the iPhone and go to More → iPhone Shortcut.
				{/if}
			</p>
		{:else}
			<ul class="keys">
				{#each keys as key (key.id)}
					<li>
						<span class="key-text">
							<strong>{key.name}</strong>
							<span class="muted">logs to {key.account_name} · {lastUsed(key.last_used_at)}</span>
						</span>
						<Button variant="danger" size="sm" disabled={revoking === key.id} onclick={() => revoke(key)}>
							{revoking === key.id ? 'Revoking…' : 'Revoke'}
						</Button>
					</li>
				{/each}
			</ul>
			{#if !onAppleMobile}
				<p class="muted small">To add another, open Fangorn on that iPhone and go to More → iPhone Shortcut.</p>
			{/if}
		{/if}
	</section>

	{#if loginRequired}
		<section class="card">
			<h2>This device</h2>
			<p class="muted small">
				Signing out here doesn't affect other phones or computers. To sign every device out, change
				the household password.
			</p>
			<div class="actions">
				<Button variant="secondary" onclick={signOut}>Sign out</Button>
			</div>
		</section>
	{/if}
</div>

<style>
	h2 {
		font-size: 1rem;
	}

	.section-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		margin-bottom: 0.5rem;
	}

	section > h2 {
		margin-bottom: 0.5rem;
	}

	.small {
		font-size: 0.875rem;
		line-height: 1.5;
	}

	.default-account {
		margin-top: 0.75rem;
		width: 100%;
		max-width: 24rem;
	}

	.setup {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ink);
		text-decoration: none;
		padding: 0.375rem 0.75rem;
		border: 1px solid var(--accent);
		border-radius: var(--radius-sm);
		white-space: nowrap;
	}

	.empty-note {
		margin-top: 0.75rem;
		color: var(--muted);
	}

	.keys {
		list-style: none;
		margin: 0.75rem 0;
		border-top: 1px solid var(--divider);
	}

	.keys li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.625rem 0;
		border-bottom: 1px solid var(--divider);
	}

	.key-text {
		display: flex;
		flex-direction: column;
		line-height: 1.35;
		min-width: 0;
	}

	.key-text .muted {
		font-size: 0.8125rem;
	}

	.actions {
		margin-top: 0.75rem;
	}
</style>
