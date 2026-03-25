<script lang="ts">
	import { onMount } from 'svelte';
	import { getTransactions, getAccounts } from '$lib/api';
	import type { Transaction, Account } from '$lib/types';
	import TransactionRow from '$lib/components/TransactionRow.svelte';

	let transactions: Transaction[] = $state([]);
	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	let search = $state('');
	let selectedAccount = $state('');
	let dateFrom = $state('');
	let dateTo = $state('');

	async function load() {
		try {
			loading = true;
			error = null;
			transactions = await getTransactions({
				search: search || undefined,
				account_id: selectedAccount ? Number(selectedAccount) : undefined,
				from: dateFrom || undefined,
				to: dateTo || undefined,
				limit: 200
			});
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load transactions';
		} finally {
			loading = false;
		}
	}

	onMount(async () => {
		accounts = await getAccounts().catch(() => []);
		await load();
	});

	function formatCurrency(n: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n);
	}
</script>

<svelte:head>
	<title>Fangorn - Transactions</title>
</svelte:head>

<div class="transactions-page">
	<h1>Transactions</h1>

	<div class="filters">
		<input type="text" placeholder="Search..." bind:value={search} onchange={load} />
		<select bind:value={selectedAccount} onchange={load}>
			<option value="">All Accounts</option>
			{#each accounts as account}
				<option value={account.id}>{account.name}</option>
			{/each}
		</select>
		<input type="date" bind:value={dateFrom} onchange={load} />
		<input type="date" bind:value={dateTo} onchange={load} />
	</div>

	{#if loading}
		<p class="status">Loading...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if transactions.length === 0}
		<p class="status">No transactions found.</p>
	{:else}
		<div class="transaction-list">
			<div class="list-header">
				<span class="col-date">Date</span>
				<span class="col-name">Description</span>
				<span class="col-category">Category</span>
				<span class="col-amount">Amount</span>
			</div>
			{#each transactions as txn}
				<TransactionRow transaction={txn} />
			{/each}
		</div>
	{/if}
</div>

<style>
	.transactions-page {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	h1 {
		font-size: 1.75rem;
		font-weight: 700;
	}

	.filters {
		display: flex;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.filters input,
	.filters select {
		padding: 0.5rem 0.75rem;
		border: 1px solid #ddd;
		border-radius: 8px;
		font-size: 0.9rem;
		background: white;
	}

	.filters input[type='text'] {
		flex: 1;
		min-width: 200px;
	}

	.transaction-list {
		background: white;
		border-radius: 12px;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
		overflow: hidden;
	}

	.list-header {
		display: grid;
		grid-template-columns: 100px 1fr 150px 120px;
		padding: 0.75rem 1rem;
		background: #f8f9fa;
		font-size: 0.8rem;
		font-weight: 600;
		color: #666;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.col-amount {
		text-align: right;
	}

	.status {
		text-align: center;
		padding: 3rem;
		color: #666;
	}

	.status.error {
		color: #ef4444;
	}
</style>
