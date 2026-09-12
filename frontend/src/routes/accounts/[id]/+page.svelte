<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getAccount } from '$lib/api';
	import type { AccountDetail } from '$lib/types';
	import { ACCOUNT_TYPE_LABELS } from '$lib/types';
	import { formatCurrency, formatDate } from '$lib/format';
	import TransactionRow from '$lib/components/TransactionRow.svelte';

	let detail = $state<AccountDetail | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let accountId = $derived(Number(page.params.id));

	onMount(load);

	async function load() {
		loading = true;
		error = null;
		try {
			detail = await getAccount(accountId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load this account';
		} finally {
			loading = false;
		}
	}

	let isLiability = $derived(detail?.account.class === 'liability');
</script>

<div class="page">
	<a class="back" href="/accounts">← Accounts</a>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if detail}
		{@const account = detail.account}
		<div class="card header-card">
			<div>
				<h1>{account.name}</h1>
				<p class="muted">
					{ACCOUNT_TYPE_LABELS[account.type] ?? account.type}
					{#if account.institution_name}· {account.institution_name}{/if}
					{#if account.mask}· ····{account.mask}{/if}
				</p>
			</div>
			<div class="balance-block">
				<span class="muted">{isLiability ? 'Owed' : 'Balance'}</span>
				<span class="balance" class:neg={isLiability && account.balance !== 0}>
					{formatCurrency(isLiability ? Math.abs(account.balance) : account.balance)}
				</span>
				<span class="muted small">
					Started at {formatCurrency(Math.abs(account.starting_balance))} on
					{formatDate(account.starting_balance_date)}
				</span>
			</div>
		</div>

		<div class="card">
			<h2>Register</h2>
			{#if detail.transactions.length === 0}
				<p class="muted small">
					Nothing logged yet. The balance above is still the starting balance you entered.
				</p>
			{:else}
				<div class="table-scroll">
					<div class="table">
						<div class="head">
							<span>Date</span>
							<span>Description</span>
							<span>Category</span>
							<span class="right">Amount</span>
							<span class="right">Balance</span>
						</div>
						{#each detail.transactions as transaction (transaction.id)}
							<TransactionRow {transaction} showAccount={false} showRunningBalance={true} />
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.back {
		font-size: 0.875rem;
		color: var(--muted);
		text-decoration: none;
		width: fit-content;
	}

	.back:hover {
		color: var(--ink);
	}

	h1 {
		font-size: 1.5rem;
	}

	h2 {
		font-size: 1rem;
		margin-bottom: 1rem;
	}

	.header-card {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1.5rem;
		flex-wrap: wrap;
	}

	.balance-block {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		text-align: right;
	}

	.balance {
		font-size: 2rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		line-height: 1.2;
	}

	.small {
		font-size: 0.75rem;
	}

	.table-scroll {
		overflow-x: auto;
	}

	.table {
		min-width: 640px;
	}

	.head {
		display: grid;
		grid-template-columns: 80px 1fr 150px 120px 120px;
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
</style>
