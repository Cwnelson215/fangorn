<script lang="ts">
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	// The quick-log screen: what the home-screen icon and the tab bar's + open.
	// Everything that can be defaulted is — today, the last account used, a
	// description from the category — so the common case is amount, category,
	// Save. A receipt photo is the other way in and gets the same prominence:
	// one tap on the camera and it logs itself. It also takes a prefill from the URL (?amount=&kind=&category=&note=)
	// so a phone shortcut can open it half filled in.
	import { onMount, tick } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		createTransaction,
		deleteTransaction,
		getAccounts,
		getCategories,
		getTransactions
	} from '$lib/api';
	import type { Account, Category, Transaction } from '$lib/types';
	import { formatCurrency, formatDateShort, today } from '$lib/format';
	import { pickRemembered, rememberId } from '$lib/remember';
	import { capture, openCamera } from '$lib/capture.svelte';

	type Kind = 'expense' | 'income' | 'refund';

	const KINDS: { value: Kind; label: string }[] = [
		{ value: 'expense', label: 'Money out' },
		{ value: 'income', label: 'Money in' },
		{ value: 'refund', label: 'Refund' }
	];

	let accounts = $state<Account[]>([]);
	let categories = $state<Category[]>([]);
	// Category id -> how often it was used recently, so the usual ones come first.
	let usage = $state(new Map<number, number>());
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	let kind = $state<Kind>('expense');
	let amountText = $state('');
	let categoryId = $state(0);
	let accountId = $state(0);
	// Set while the account was picked by the category ("gas goes on the Visa")
	// rather than by hand: the account to go back to if the category is unpicked.
	// A routed account isn't remembered as this phone's usual one.
	let routedFrom = $state<number | null>(null);
	let date = $state(today());
	let note = $state('');

	let saving = $state(false);
	let error = $state<string | null>(null);
	let saved = $state<Transaction | null>(null);
	let undoing = $state(false);

	let amountInput = $state<HTMLInputElement>();

	// Accepts what people type on a phone: "12", "12.5", "$1,200.00".
	let amount = $derived.by(() => {
		const n = Number(amountText.replace(/[$,\s]/g, ''));
		return Number.isFinite(n) && n > 0 ? Math.round(n * 100) / 100 : 0;
	});

	let side = $derived(kind === 'refund' ? 'expense' : kind);
	let choices = $derived(
		categories
			.filter((c) => c.kind === side)
			.sort((a, b) => (usage.get(b.id) ?? 0) - (usage.get(a.id) ?? 0) || a.name.localeCompare(b.name))
	);
	let category = $derived(categories.find((c) => c.id === categoryId));
	// The usual handful up front; the rest one tap away. A selection made from
	// the rest stays visible after it's collapsed.
	const FEW = 8;
	let showAll = $state(false);
	let visible = $derived(
		showAll || choices.length <= FEW + 1
			? choices
			: choices.filter((c, i) => i < FEW || c.id === categoryId)
	);
	let canSave = $derived(amount > 0 && accountId > 0 && (kind !== 'refund' || categoryId > 0) && !saving);

	onMount(async () => {
		try {
			const [a, c, recent] = await Promise.all([
				getAccounts(),
				getCategories(),
				getTransactions({ limit: 200 }).catch(() => [] as Transaction[])
			]);
			accounts = a;
			categories = c;
			const counts = new Map<number, number>();
			for (const t of recent) {
				if (t.category_id) counts.set(t.category_id, (counts.get(t.category_id) ?? 0) + 1);
			}
			usage = counts;
			accountId = pickRemembered('transaction.account', accounts, accounts[0]?.id ?? 0);
			applyPrefill();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load your accounts';
		} finally {
			loading = false;
		}
		await tick();
		// Android opens the keyboard for this; iOS only does on a tap, which is
		// why the amount field is the biggest thing on the screen.
		amountInput?.focus();
	});

	function applyPrefill() {
		const q = page.url.searchParams;
		const k = q.get('kind');
		if (k === 'expense' || k === 'income' || k === 'refund') kind = k;
		if (q.get('amount')) amountText = q.get('amount') ?? '';
		const name = q.get('category')?.trim().toLowerCase();
		if (name) {
			const match = categories.find((c) => c.name.toLowerCase() === name && c.kind === side);
			if (match) chooseCategory(match.id);
		}
		if (q.get('note')) note = q.get('note') ?? '';
		if ([...q.keys()].length > 0) goto('/add', { replaceState: true, noScroll: true, keepFocus: true });
	}

	function setKind(next: Kind) {
		kind = next;
		// Switching between money in and out strands a category from the other list.
		if (!choices.some((c) => c.id === categoryId)) chooseCategory(0);
	}

	// Picks a category (0 for none) and follows its account, if it names one
	// that is still open.
	function chooseCategory(id: number) {
		categoryId = id;
		const target = categories.find((c) => c.id === id)?.default_account_id;
		if (target && accounts.some((a) => a.id === target)) {
			if (routedFrom === null) routedFrom = accountId;
			accountId = target;
		} else if (routedFrom !== null) {
			accountId = routedFrom;
			routedFrom = null;
		}
	}

	let routedAccount = $derived(
		routedFrom !== null ? accounts.find((a) => a.id === accountId) : undefined
	);

	async function save(event: Event) {
		event.preventDefault();
		if (!canSave) return;
		saving = true;
		error = null;
		try {
			const description =
				note.trim() || category?.name || (kind === 'income' ? 'Income' : kind === 'refund' ? 'Refund' : 'Expense');
			saved = await createTransaction({
				account_id: accountId,
				date,
				amount,
				kind,
				description,
				merchant: null,
				category_id: categoryId || null,
				notes: null
			});
			if (routedFrom === null) rememberId('transaction.account', accountId);
			// Ready for the next one; account, kind and date carry over — except
			// an account the category chose, which goes back.
			amountText = '';
			chooseCategory(0);
			note = '';
			amountInput?.focus();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save that';
		} finally {
			saving = false;
		}
	}

	async function undo() {
		if (!saved) return;
		undoing = true;
		try {
			await deleteTransaction(saved.id);
			// Put it back in the form so a typo can be fixed and saved again.
			kind = saved.kind as Kind;
			amountText = String(Math.abs(saved.amount));
			chooseCategory(saved.category_id ?? 0);
			note = saved.description === category?.name ? '' : saved.description;
			accountId = saved.account_id;
			date = saved.date;
			saved = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not undo that';
		} finally {
			undoing = false;
		}
	}

	let savedAccount = $derived(accounts.find((a) => a.id === saved?.account_id));
</script>

<svelte:head>
	<title>Log · Fangorn</title>
</svelte:head>

<div class="quick">
	{#if loading}
		<p class="muted center">Loading…</p>
	{:else if loadError}
		<p class="error-text center">{loadError}</p>
	{:else if accounts.length === 0}
		<div class="card empty">
			<h2>Add an account first</h2>
			<p class="muted">Money has to land somewhere.</p>
			<a class="link-button" href="/accounts">Go to accounts</a>
		</div>
	{:else}
		<button type="button" class="snap" onclick={openCamera} disabled={capture.stage !== null}>
			<svg viewBox="0 0 24 24" aria-hidden="true"
				><path d="M4 8h3l2-3h6l2 3h3v11H4z" /><circle cx="12" cy="13" r="3.5" /></svg
			>
			<span class="snap-text">
				<strong>{capture.stage ? 'Reading receipt…' : 'Snap a receipt'}</strong>
				<span>It logs itself — or type it in below</span>
			</span>
		</button>

		{#if saved}
			<div class="toast" role="status">
				<span>
					<strong>Logged {formatCurrency(Math.abs(saved.amount))}</strong>
					{saved.category_name ?? saved.description} · {savedAccount?.name ?? ''}
				</span>
				<button type="button" onclick={undo} disabled={undoing}>{undoing ? '…' : 'Undo'}</button>
			</div>
		{/if}

		<form onsubmit={save}>
			<div class="kinds" role="group" aria-label="Type">
				{#each KINDS as k (k.value)}
					<button type="button" class:active={kind === k.value} onclick={() => setKind(k.value)}>
						{k.label}
					</button>
				{/each}
			</div>

			<label class="amount" class:income={kind !== 'expense'}>
				<span class="currency">$</span>
				<input
					bind:this={amountInput}
					bind:value={amountText}
					type="text"
					inputmode="decimal"
					enterkeyhint="done"
					autocomplete="off"
					placeholder="0.00"
					aria-label="Amount"
				/>
			</label>

			<div class="section-label">
				{kind === 'refund' ? 'Refund of' : 'Category'}
				{#if kind === 'refund'}<span class="req">required</span>{/if}
			</div>
			<div class="chips" role="radiogroup" aria-label="Category">
				{#each visible as c (c.id)}
					<button
						type="button"
						role="radio"
						aria-checked={categoryId === c.id}
						class="chip"
						class:selected={categoryId === c.id}
						style="--chip: {c.color ?? 'var(--accent)'}"
						onclick={() => chooseCategory(categoryId === c.id ? 0 : c.id)}
					>
						<span class="dot"></span>{c.name}
					</button>
				{/each}
				{#if visible.length < choices.length}
					<button type="button" class="chip more" onclick={() => (showAll = true)}>
						+{choices.length - visible.length} more
					</button>
				{/if}
			</div>

			<input
				class="note"
				bind:value={note}
				placeholder={category ? `What for? (defaults to “${category.name}”)` : 'What for? (optional)'}
				enterkeyhint="done"
				aria-label="Description"
			/>

			<div class="meta">
				<label class="pill">
					<span class="pill-label">From</span>
					<select bind:value={accountId} aria-label="Account" onchange={() => (routedFrom = null)}>
						<AccountOptions {accounts} />
					</select>
				</label>
				<label class="pill date">
					<span class="pill-label">{date === today() ? 'Today' : formatDateShort(date)}</span>
					<input type="date" bind:value={date} aria-label="Date" />
				</label>
			</div>

			{#if routedAccount && category}
				<p class="routed">{category.name} always goes on {routedAccount.name}.</p>
			{/if}

			{#if error}
				<p class="error-text">{error}</p>
			{/if}

			<button type="submit" class="save" disabled={!canSave}>
				{saving ? 'Saving…' : amount > 0 ? `Save ${formatCurrency(amount)}` : 'Save'}
			</button>
		</form>

		<p class="aside muted">
			Moving money between your own accounts? <a href="/transfers?new">Record a transfer</a>
		</p>

	{/if}
</div>

<style>
	.routed {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--muted);
	}

	.quick {
		max-width: 480px;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.center {
		text-align: center;
		padding: 2rem 0;
	}

	.snap {
		display: flex;
		align-items: center;
		gap: 0.875rem;
		min-height: 64px;
		padding: 0.75rem 1rem;
		border: 2px solid var(--accent);
		border-radius: var(--radius);
		background: var(--surface);
		font: inherit;
		color: var(--ink);
		text-align: left;
		cursor: pointer;
		box-shadow: var(--shadow);
	}

	.snap:active:not(:disabled) {
		background: #e6f8f1;
	}

	.snap:disabled {
		cursor: progress;
		opacity: 0.75;
	}

	.snap svg {
		flex: none;
		width: 40px;
		height: 40px;
		padding: 8px;
		border-radius: 50%;
		background: var(--accent);
		fill: none;
		stroke: var(--ink);
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.snap-text {
		display: flex;
		flex-direction: column;
		line-height: 1.3;
	}

	.snap-text strong {
		font-size: 1.0625rem;
	}

	.snap-text span {
		font-size: 0.8125rem;
		color: var(--muted);
	}

	.aside {
		text-align: center;
		font-size: 0.875rem;
	}

	.aside a {
		color: var(--ink);
		font-weight: 600;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		background: var(--surface);
		border-radius: var(--radius);
		box-shadow: var(--shadow);
		padding: 1rem;
	}

	.kinds {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		background: var(--bg);
		border-radius: var(--radius-sm);
		padding: 0.25rem;
		gap: 0.25rem;
	}

	.kinds button {
		min-height: 40px;
		border: none;
		background: none;
		border-radius: 6px;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--muted);
		cursor: pointer;
	}

	.kinds button.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow);
	}

	.amount {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.25rem;
		padding: 0.5rem 0;
		color: var(--neg);
		cursor: text;
	}

	.amount.income {
		color: var(--pos);
	}

	.currency {
		font-size: 2rem;
		font-weight: 600;
	}

	.amount input {
		width: 100%;
		max-width: 12ch;
		border: none;
		outline: none;
		background: none;
		font: inherit;
		font-size: 3rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		color: inherit;
		text-align: left;
		padding: 0;
	}

	.amount input::placeholder {
		color: var(--border);
	}

	.section-label {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--muted);
		margin-bottom: -0.5rem;
	}

	.req {
		font-weight: 500;
		color: var(--muted-light);
		margin-left: 0.25rem;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.chip {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		min-height: 40px;
		padding: 0 0.875rem;
		border: 1px solid var(--border);
		border-radius: 999px;
		background: var(--surface);
		font: inherit;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--ink);
		cursor: pointer;
	}

	.dot {
		width: 0.625rem;
		height: 0.625rem;
		border-radius: 50%;
		background: var(--chip);
	}

	.chip.more {
		color: var(--muted);
		border-style: dashed;
	}

	.chip.selected {
		border-color: var(--ink);
		background: var(--ink);
		color: white;
	}

	.note {
		width: 100%;
		min-height: 44px;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		font: inherit;
		font-size: 1rem;
	}

	.note:focus {
		outline: none;
		border-color: var(--accent);
	}

	.meta {
		display: flex;
		gap: 0.5rem;
	}

	/* The label shows the value; the native control sits invisibly on top so a
	   tap opens the phone's own picker. */
	.pill {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.375rem;
		min-height: 40px;
		padding: 0 0.875rem;
		border-radius: 999px;
		background: var(--bg);
		font-size: 0.875rem;
		font-weight: 600;
		min-width: 0;
	}

	.pill:first-child {
		flex: 1;
	}

	.pill select {
		flex: 1;
		min-width: 0;
		border: none;
		background: none;
		font: inherit;
		font-size: 1rem;
		color: var(--ink);
		appearance: none;
		text-overflow: ellipsis;
		cursor: pointer;
	}

	.pill-label {
		color: var(--muted);
		white-space: nowrap;
	}

	.pill.date .pill-label {
		color: var(--ink);
	}

	.pill.date input {
		position: absolute;
		inset: 0;
		opacity: 0;
		width: 100%;
		cursor: pointer;
	}

	/* Pinned above the tab bar so it's reachable without scrolling past the
	   categories, keyboard up or not. */
	.save {
		position: sticky;
		bottom: calc(64px + env(safe-area-inset-bottom) + 0.75rem);
		box-shadow: 0 4px 16px rgba(78, 204, 163, 0.35);
		min-height: 52px;
		border: none;
		border-radius: var(--radius-sm);
		background: var(--accent);
		font: inherit;
		font-size: 1.0625rem;
		font-weight: 700;
		color: var(--ink);
		cursor: pointer;
	}

	.save:disabled {
		background: #bfeedd;
		box-shadow: none;
		cursor: not-allowed;
	}

	@media (min-width: 900px) {
		.save {
			position: static;
		}
	}

	.toast {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
		border-radius: var(--radius-sm);
		background: var(--ink);
		color: white;
		font-size: 0.875rem;
		animation: pop 0.2s ease-out;
	}

	.toast strong {
		color: var(--accent);
		margin-right: 0.25rem;
	}

	.toast button {
		min-height: 36px;
		padding: 0 0.75rem;
		border: 1px solid rgba(255, 255, 255, 0.3);
		border-radius: var(--radius-sm);
		background: none;
		font: inherit;
		font-weight: 600;
		color: white;
		cursor: pointer;
	}

	.link-button {
		display: inline-block;
		margin-top: 1rem;
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		background: var(--accent);
		color: var(--ink);
		font-weight: 600;
		text-decoration: none;
	}

	@keyframes pop {
		from {
			transform: translateY(8px);
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.toast {
			animation: none;
		}
	}
</style>
