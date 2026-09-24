<script lang="ts">
	import { onMount } from 'svelte';
	import {
		createTransaction,
		deleteTransaction,
		getAccounts,
		getCategories,
		getTransactions,
		receiptImageUrl,
		updateTransaction,
		type TransactionQuery
	} from '$lib/api';
	import type { Account, Category, Transaction, TransactionInput } from '$lib/types';
	import { formatDayHeading, groupByDate, today } from '$lib/format';
	import { pickRemembered, rememberId } from '$lib/remember';
	import TransactionRow from '$lib/components/TransactionRow.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

	let transactions: Transaction[] = $state([]);
	let accounts: Account[] = $state([]);
	let categories: Category[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	// Filters
	let search = $state('');
	let filterAccount = $state(0);
	let filterCategory = $state(0);
	let filterKind = $state('');
	let dateFrom = $state('');
	let dateTo = $state('');
	// On a phone only search shows until the rest are asked for.
	let filtersOpen = $state(false);
	let activeFilters = $derived(
		[filterAccount, filterCategory, filterKind, dateFrom, dateTo].filter(Boolean).length
	);
	let days = $derived(groupByDate(transactions, (t) => t.date));

	// Entry form
	let modalOpen = $state(false);
	let editing = $state<Transaction | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let kind = $state<'income' | 'expense' | 'refund'>('expense');
	let accountId = $state(0);
	let date = $state(today());
	let amount = $state('');
	let description = $state('');
	let merchant = $state('');
	let categoryId = $state(0);
	let notes = $state('');

	onMount(async () => {
		try {
			[accounts, categories] = await Promise.all([getAccounts(), getCategories()]);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load accounts';
		}
		await load();
	});

	async function load() {
		loading = true;
		loadError = null;
		try {
			const query: TransactionQuery = {
				search: search || undefined,
				account_id: filterAccount || undefined,
				category_id: filterCategory || undefined,
				kind: filterKind || undefined,
				from: dateFrom || undefined,
				to: dateTo || undefined,
				limit: 200
			};
			transactions = await getTransactions(query);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load transactions';
		} finally {
			loading = false;
		}
	}

	// Only categories matching the selected direction are offered, so an expense
	// can't be filed under "Paycheck". A refund points back at what was spent, so
	// it picks from the same list an expense does.
	let categorySide = $derived(kind === 'refund' ? 'expense' : kind);
	let availableCategories = $derived(categories.filter((c) => c.kind === categorySide));

	function openCreate() {
		editing = null;
		kind = 'expense';
		accountId = pickRemembered('transaction.account', accounts, accounts[0]?.id ?? 0);
		date = today();
		amount = '';
		description = '';
		merchant = '';
		categoryId = 0;
		notes = '';
		formError = null;
		modalOpen = true;
	}

	// Income, expenses and refunds are edited here. A transfer is a linked pair,
	// and a trade's cash side belongs to its trade — both have their own screens.
	function isEditable(transaction: Transaction): boolean {
		return (
			transaction.kind === 'income' ||
			transaction.kind === 'expense' ||
			transaction.kind === 'refund'
		);
	}

	function openEdit(transaction: Transaction) {
		if (!isEditable(transaction)) return;

		editing = transaction;
		kind = transaction.kind as 'income' | 'expense' | 'refund';
		accountId = transaction.account_id;
		date = transaction.date;
		amount = String(Math.abs(transaction.amount));
		description = transaction.description;
		merchant = transaction.merchant ?? '';
		categoryId = transaction.category_id ?? 0;
		notes = transaction.notes ?? '';
		formError = null;
		modalOpen = true;
	}

	function buildInput(): TransactionInput {
		return {
			account_id: accountId,
			date,
			// The server owns the sign; the form always sends a positive magnitude.
			amount: Math.abs(parseFloat(amount) || 0),
			kind,
			description: description.trim(),
			merchant: merchant.trim() || null,
			category_id: categoryId || null,
			notes: notes.trim() || null
		};
	}

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (!accountId || !amount || !description.trim()) return;
		// The server rejects this too; catching it here names the missing piece
		// instead of bouncing the whole form back.
		if (kind === 'refund' && !categoryId) {
			formError = 'Pick the category the money is coming back from.';
			return;
		}

		saving = true;
		formError = null;
		try {
			if (editing) {
				await updateTransaction(editing.id, buildInput());
			} else {
				await createTransaction(buildInput());
				// Only a new entry sets the default; fixing an old one shouldn't.
				rememberId('transaction.account', accountId);
			}
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the transaction';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!editing) return;
		deleting = true;
		formError = null;
		try {
			await deleteTransaction(editing.id);
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete the transaction';
		} finally {
			deleting = false;
		}
	}

	// Switching direction can strand a category from the other list.
	function onKindChange() {
		if (!availableCategories.some((c) => c.id === categoryId)) categoryId = 0;
	}
</script>

<div class="page">
	<div class="page-header">
		<h1>Transactions</h1>
		<span class="header-actions">
			<a class="scan" href="/receipts">Scan receipt</a>
			<Button onclick={openCreate} disabled={accounts.length === 0}>Log Transaction</Button>
		</span>
	</div>

	{#if accounts.length === 0 && !loading}
		<div class="card empty">
			<h2>Add an account first</h2>
			<p class="muted">Transactions have to land somewhere.</p>
			<a class="cta" href="/accounts">Go to accounts</a>
		</div>
	{:else}
		<div class="card filters" class:open={filtersOpen}>
			<div class="search-row">
				<Field label="Search" id="search">
					<input
						id="search"
						type="search"
						enterkeyhint="search"
						bind:value={search}
						placeholder="Description or merchant"
						onchange={load}
					/>
				</Field>
				<button
					type="button"
					class="filters-toggle"
					aria-expanded={filtersOpen}
					onclick={() => (filtersOpen = !filtersOpen)}
				>
					Filters{#if activeFilters > 0}<span class="badge">{activeFilters}</span>{/if}
				</button>
			</div>
			<Field label="Account" id="filterAccount">
				<select id="filterAccount" bind:value={filterAccount} onchange={load}>
					<option value={0}>All accounts</option>
					{#each accounts as account (account.id)}
						<option value={account.id}>{account.name}</option>
					{/each}
				</select>
			</Field>
			<Field label="Category" id="filterCategory">
				<select id="filterCategory" bind:value={filterCategory} onchange={load}>
					<option value={0}>All categories</option>
					{#each categories as category (category.id)}
						<option value={category.id}>{category.name}</option>
					{/each}
				</select>
			</Field>
			<Field label="Type" id="filterKind">
				<select id="filterKind" bind:value={filterKind} onchange={load}>
					<option value="">All</option>
					<option value="expense">Money out</option>
					<option value="income">Money in</option>
					<option value="refund">Refunds</option>
					<option value="transfer">Transfers</option>
					<option value="trade">Trades</option>
				</select>
			</Field>
			<Field label="From" id="dateFrom">
				<input id="dateFrom" type="date" bind:value={dateFrom} onchange={load} />
			</Field>
			<Field label="To" id="dateTo">
				<input id="dateTo" type="date" bind:value={dateTo} onchange={load} />
			</Field>
		</div>

		<div class="card">
			{#if loading}
				<p class="muted">Loading…</p>
			{:else if loadError}
				<p class="error-text">{loadError}</p>
			{:else if transactions.length === 0}
				<p class="muted small">No transactions match these filters.</p>
			{:else}
				<div class="table-scroll">
					<div class="table">
						<div class="head table-head">
							<span>Date</span>
							<span>Description</span>
							<span>Category</span>
							<span class="right">Amount</span>
						</div>
						{#each days as day (day.date)}
							<div class="day-heading">{formatDayHeading(day.date)}</div>
							{#each day.items as transaction (transaction.id)}
								<TransactionRow
									{transaction}
									onedit={isEditable(transaction) ? openEdit : undefined}
								/>
							{/each}
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>

<Modal bind:open={modalOpen} title={editing ? 'Edit Transaction' : 'Log Transaction'}>
	<form onsubmit={handleSubmit}>
		<div class="toggle">
			<button
				type="button"
				class:active={kind === 'expense'}
				onclick={() => {
					kind = 'expense';
					onKindChange();
				}}
			>
				Money out
			</button>
			<button
				type="button"
				class:active={kind === 'income'}
				onclick={() => {
					kind = 'income';
					onKindChange();
				}}
			>
				Money in
			</button>
			<button
				type="button"
				class:active={kind === 'refund'}
				onclick={() => {
					kind = 'refund';
					onKindChange();
				}}
			>
				Refund
			</button>
		</div>

		{#if kind === 'refund'}
			<p class="hint muted">
				Money coming back from something you already logged. It lands in the account like income,
				but comes off that category's spending instead of counting as earnings.
			</p>
		{/if}

		<div class="form-row">
			<Field label="Amount" id="amount">
				<input
					id="amount"
					type="number"
					inputmode="decimal"
					step="0.01"
					min="0.01"
					placeholder="0.00"
					bind:value={amount}
					disabled={saving}
					required
				/>
			</Field>
			<Field label="Date" id="date">
				<input id="date" type="date" bind:value={date} disabled={saving} required />
			</Field>
		</div>

		<Field label="Description" id="description">
			<input
				id="description"
				bind:value={description}
				placeholder={kind === 'expense' ? 'Groceries' : kind === 'refund' ? 'Returned groceries' : 'Paycheck'}
				disabled={saving}
				required
			/>
		</Field>

		<div class="form-row">
			<Field label="Account" id="account">
				<select id="account" bind:value={accountId} disabled={saving}>
					{#each accounts as account (account.id)}
						<option value={account.id}>{account.name}</option>
					{/each}
				</select>
			</Field>
			<Field label={kind === 'refund' ? 'Refund of' : 'Category'} id="category">
				<select id="category" bind:value={categoryId} disabled={saving} required={kind === 'refund'}>
					<option value={0}>{kind === 'refund' ? 'Pick a category' : 'Uncategorized'}</option>
					{#each availableCategories as category (category.id)}
						<option value={category.id}>{category.name}</option>
					{/each}
				</select>
			</Field>
		</div>

		<Field label="Merchant" id="merchant">
			<input id="merchant" bind:value={merchant} placeholder="Optional" disabled={saving} />
		</Field>

		<Field label="Notes" id="txnNotes">
			<textarea id="txnNotes" bind:value={notes} disabled={saving}></textarea>
		</Field>

		{#if editing?.receipt_id}
			<p class="hint muted">
				Posted from a receipt ·
				<a href={receiptImageUrl(editing.receipt_id)} target="_blank" rel="noopener">view the photo</a>.
				Deleting this transaction deletes the photo too.
			</p>
		{/if}

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			{#if editing}
				<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
					{deleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (modalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !amount || !description.trim()}>
				{saving ? 'Saving…' : editing ? 'Save Changes' : 'Log It'}
			</Button>
		</div>
	</form>
</Modal>

<style>
	h2 {
		font-size: 1rem;
		margin-bottom: 0.5rem;
	}

	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: 1rem;
	}

	/* On a desktop the search row dissolves into the grid like any other field. */
	.search-row {
		display: contents;
	}

	.filters-toggle {
		display: none;
	}

	.table-scroll {
		overflow-x: auto;
	}

	.table {
		min-width: 560px;
	}

	.head {
		display: grid;
		grid-template-columns: 80px 1fr 150px 120px;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		background: var(--bg);
		border-radius: var(--radius-sm);
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.right {
		text-align: right;
	}

	.small {
		font-size: 0.875rem;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.form-row {
		display: flex;
		gap: 1rem;
	}

	.header-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.scan {
		font-weight: 600;
		font-size: 0.9rem;
		color: var(--ink);
		text-decoration: none;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--surface);
	}

	.scan:hover {
		border-color: var(--accent);
	}

	.form-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.spacer {
		flex: 1;
	}

	.hint {
		margin: 0.75rem 0 0;
		font-size: 0.8rem;
		line-height: 1.4;
	}

	.toggle {
		display: flex;
		background: var(--bg);
		border-radius: var(--radius-sm);
		padding: 0.25rem;
		gap: 0.25rem;
	}

	.toggle button {
		flex: 1;
		padding: 0.5rem;
		border: none;
		background: none;
		border-radius: 6px;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--muted);
		cursor: pointer;
	}

	.toggle button.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow);
	}

	@media (max-width: 639px) {
		.filters {
			grid-template-columns: 1fr 1fr;
			gap: 0.75rem;
		}

		.search-row {
			display: flex;
			align-items: flex-end;
			gap: 0.5rem;
			grid-column: 1 / -1;
		}

		.filters > :global(.field) {
			display: none;
		}

		.filters.open > :global(.field) {
			display: flex;
		}

		/* The search box is labelled by its placeholder on a phone. */
		.search-row :global(label) {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip: rect(0 0 0 0);
		}

		.filters-toggle {
			display: inline-flex;
			align-items: center;
			gap: 0.375rem;
			min-height: 44px;
			padding: 0 0.875rem;
			border: 1px solid var(--border);
			border-radius: var(--radius-sm);
			background: var(--surface);
			font: inherit;
			font-size: 0.9375rem;
			font-weight: 600;
			color: var(--ink);
			cursor: pointer;
		}

		.filters-toggle[aria-expanded='true'] {
			border-color: var(--accent);
		}

		.badge {
			min-width: 1.25rem;
			padding: 0 0.3rem;
			border-radius: 999px;
			background: var(--accent);
			font-size: 0.75rem;
			line-height: 1.25rem;
			text-align: center;
		}

		/* Logging lives on the tab bar's + (the quick-log screen) on a phone. */
		.header-actions {
			display: none;
		}
	}

	.cta {
		display: inline-block;
		background: var(--accent);
		color: var(--ink);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
		margin-top: 1rem;
	}
</style>
