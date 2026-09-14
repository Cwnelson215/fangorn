<script lang="ts">
	import { onMount } from 'svelte';
	import {
		createTransaction,
		deleteTransaction,
		getAccounts,
		getCategories,
		getTransactions,
		updateTransaction,
		type TransactionQuery
	} from '$lib/api';
	import type { Account, Category, Transaction, TransactionInput } from '$lib/types';
	import { today } from '$lib/format';
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

	// Entry form
	let modalOpen = $state(false);
	let editing = $state<Transaction | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let kind = $state<'income' | 'expense'>('expense');
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
	// can't be filed under "Paycheck".
	let availableCategories = $derived(categories.filter((c) => c.kind === kind));

	function openCreate() {
		editing = null;
		kind = 'expense';
		accountId = accounts[0]?.id ?? 0;
		date = today();
		amount = '';
		description = '';
		merchant = '';
		categoryId = 0;
		notes = '';
		formError = null;
		modalOpen = true;
	}

	// Only plain income and expenses are edited here. A transfer is a linked pair,
	// and a trade's cash side belongs to its trade — both have their own screens.
	function isEditable(transaction: Transaction): boolean {
		return transaction.kind === 'income' || transaction.kind === 'expense';
	}

	function openEdit(transaction: Transaction) {
		if (transaction.kind !== 'income' && transaction.kind !== 'expense') return;

		editing = transaction;
		kind = transaction.kind;
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

		saving = true;
		formError = null;
		try {
			if (editing) {
				await updateTransaction(editing.id, buildInput());
			} else {
				await createTransaction(buildInput());
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
		<Button onclick={openCreate} disabled={accounts.length === 0}>Log Transaction</Button>
	</div>

	{#if accounts.length === 0 && !loading}
		<div class="card empty">
			<h2>Add an account first</h2>
			<p class="muted">Transactions have to land somewhere.</p>
			<a class="cta" href="/accounts">Go to accounts</a>
		</div>
	{:else}
		<div class="card filters">
			<Field label="Search" id="search">
				<input id="search" bind:value={search} placeholder="Description or merchant" onchange={load} />
			</Field>
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
						<div class="head">
							<span>Date</span>
							<span>Description</span>
							<span>Category</span>
							<span class="right">Amount</span>
						</div>
						{#each transactions as transaction (transaction.id)}
							<TransactionRow
								{transaction}
								onedit={isEditable(transaction) ? openEdit : undefined}
							/>
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
		</div>

		<div class="form-row">
			<Field label="Amount" id="amount">
				<input
					id="amount"
					type="number"
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
				placeholder={kind === 'expense' ? 'Groceries' : 'Paycheck'}
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
			<Field label="Category" id="category">
				<select id="category" bind:value={categoryId} disabled={saving}>
					<option value={0}>Uncategorized</option>
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

	.form-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.spacer {
		flex: 1;
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
