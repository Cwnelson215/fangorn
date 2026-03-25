<script lang="ts">
	import type { Account } from '$lib/types';

	let { account }: { account: Account } = $props();

	function formatCurrency(n: number | null): string {
		if (n === null) return '--';
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n);
	}

	const typeIcons: Record<string, string> = {
		depository: 'bank',
		credit: 'credit_card',
		loan: 'payments',
		investment: 'trending_up'
	};
</script>

<div class="account-card">
	<div class="card-header">
		<div class="account-info">
			<div class="account-name">{account.name}</div>
			<div class="institution">{account.institution_name || ''} {account.mask ? `****${account.mask}` : ''}</div>
		</div>
		<div class="account-type">{account.subtype || account.type}</div>
	</div>
	<div class="card-body">
		<div class="balance">
			<span class="balance-label">Current</span>
			<span class="balance-value">{formatCurrency(account.current_balance)}</span>
		</div>
		{#if account.available_balance !== null && account.type === 'depository'}
			<div class="balance">
				<span class="balance-label">Available</span>
				<span class="balance-value secondary">{formatCurrency(account.available_balance)}</span>
			</div>
		{/if}
	</div>
</div>

<style>
	.account-card {
		background: white;
		border-radius: 12px;
		padding: 1.25rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
		transition: box-shadow 0.2s;
	}

	.account-card:hover {
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 1rem;
	}

	.account-name {
		font-weight: 600;
		font-size: 1rem;
	}

	.institution {
		font-size: 0.8rem;
		color: #999;
		margin-top: 0.15rem;
	}

	.account-type {
		font-size: 0.75rem;
		background: #f0f0f0;
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		color: #666;
		text-transform: capitalize;
	}

	.card-body {
		display: flex;
		gap: 1.5rem;
	}

	.balance {
		display: flex;
		flex-direction: column;
	}

	.balance-label {
		font-size: 0.75rem;
		color: #999;
	}

	.balance-value {
		font-size: 1.25rem;
		font-weight: 700;
	}

	.balance-value.secondary {
		font-size: 1rem;
		font-weight: 500;
		color: #666;
	}
</style>
