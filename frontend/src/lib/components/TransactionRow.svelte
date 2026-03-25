<script lang="ts">
	import type { Transaction } from '$lib/types';

	let { transaction }: { transaction: Transaction } = $props();

	function formatCurrency(n: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(Math.abs(n));
	}

	function formatDate(dateStr: string): string {
		const date = new Date(dateStr + 'T00:00:00');
		return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}
</script>

<div class="transaction-row" class:pending={transaction.pending}>
	<span class="col-date">{formatDate(transaction.date)}</span>
	<span class="col-name">
		<span class="name">{transaction.merchant_name || transaction.name}</span>
		{#if transaction.merchant_name && transaction.merchant_name !== transaction.name}
			<span class="subtext">{transaction.name}</span>
		{/if}
	</span>
	<span class="col-category">
		{#if transaction.category}
			<span class="category-tag">{transaction.category}</span>
		{:else if transaction.plaid_category}
			<span class="category-tag plaid">{transaction.plaid_category}</span>
		{/if}
	</span>
	<span class="col-amount" class:income={transaction.amount < 0} class:expense={transaction.amount > 0}>
		{transaction.amount < 0 ? '+' : '-'}{formatCurrency(transaction.amount)}
	</span>
</div>

<style>
	.transaction-row {
		display: grid;
		grid-template-columns: 100px 1fr 150px 120px;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid #f0f0f0;
		align-items: center;
		font-size: 0.9rem;
	}

	.transaction-row:last-child {
		border-bottom: none;
	}

	.transaction-row:hover {
		background: #fafafa;
	}

	.pending {
		opacity: 0.6;
	}

	.col-date {
		color: #999;
		font-size: 0.85rem;
	}

	.col-name {
		display: flex;
		flex-direction: column;
	}

	.name {
		font-weight: 500;
	}

	.subtext {
		font-size: 0.75rem;
		color: #999;
	}

	.category-tag {
		display: inline-block;
		font-size: 0.75rem;
		background: #e8f5e9;
		color: #2e7d32;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
	}

	.category-tag.plaid {
		background: #e3f2fd;
		color: #1565c0;
	}

	.col-amount {
		text-align: right;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.income {
		color: #22c55e;
	}

	.expense {
		color: #1a1a2e;
	}
</style>
