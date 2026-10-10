<script lang="ts">
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	import { onMount } from 'svelte';
	import { getAccounts, getCategories, getTransactions, getTransfer, type TransactionQuery } from '$lib/api';
	import type { Account, Category, Transaction, Transfer } from '$lib/types';
	import { formatDayHeading, groupByDate } from '$lib/format';
	import TransactionRow from '$lib/components/TransactionRow.svelte';
	import TransferModal from '$lib/components/TransferModal.svelte';
	import TransactionModal from '$lib/components/TransactionModal.svelte';
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

	function openCreate() {
		editing = null;
		modalOpen = true;
	}

	// Income, expenses and refunds are edited here, and a transfer opens its own
	// form, which edits both legs. A trade's cash side belongs to its trade, on
	// the account's page.
	function isEditable(transaction: Transaction): boolean {
		return (
			transaction.kind === 'income' ||
			transaction.kind === 'expense' ||
			transaction.kind === 'refund' ||
			(transaction.kind === 'transfer' && !!transaction.transfer_group_id)
		);
	}

	let transferOpen = $state(false);
	let editingTransfer = $state<Transfer | null>(null);

	async function openTransfer(groupId: string) {
		loadError = null;
		try {
			editingTransfer = await getTransfer(groupId);
			transferOpen = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the transfer';
		}
	}

	function openEdit(transaction: Transaction) {
		if (!isEditable(transaction)) return;
		if (transaction.kind === 'transfer') {
			openTransfer(transaction.transfer_group_id!);
			return;
		}

		editing = transaction;
		modalOpen = true;
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
					<AccountOptions {accounts} />
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

<TransferModal {accounts} transfer={editingTransfer} bind:open={transferOpen} onsaved={load} />

<TransactionModal {accounts} {categories} transaction={editing} bind:open={modalOpen} onsaved={load} />

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
			color: var(--on-accent);
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
		color: var(--on-accent);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
		margin-top: 1rem;
	}
</style>
