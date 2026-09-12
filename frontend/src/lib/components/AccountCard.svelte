<script lang="ts">
	import type { Account } from '$lib/types';
	import { ACCOUNT_TYPE_LABELS } from '$lib/types';
	import { formatCurrency, formatDate } from '$lib/format';

	let { account }: { account: Account } = $props();

	// Liability balances are stored negative. Showing "−$412.50" for a credit card
	// reads as though you owe negative money, so the magnitude is displayed and
	// the sign is conveyed by the "owed" label instead.
	let isLiability = $derived(account.class === 'liability');
	let displayBalance = $derived(isLiability ? Math.abs(account.balance) : account.balance);
</script>

<a class="account-card" class:archived={account.archived} href="/accounts/{account.id}">
	<div class="card-header">
		<div class="account-info">
			<div class="account-name">{account.name}</div>
			<div class="institution">
				{account.institution_name || ''}{account.mask ? ` ····${account.mask}` : ''}
			</div>
		</div>
		<div class="account-type">{ACCOUNT_TYPE_LABELS[account.type] ?? account.type}</div>
	</div>

	<div class="card-body">
		<span class="balance-label">{isLiability ? 'Owed' : 'Balance'}</span>
		<span class="balance-value" class:debt={isLiability && account.balance !== 0}>
			{formatCurrency(displayBalance)}
		</span>
		<span class="since">
			from {formatCurrency(Math.abs(account.starting_balance))} on
			{formatDate(account.starting_balance_date)}
		</span>
	</div>

	{#if account.archived}
		<span class="badge">Archived</span>
	{/if}
</a>

<style>
	.account-card {
		display: block;
		position: relative;
		background: var(--surface);
		border-radius: var(--radius);
		padding: 1.25rem;
		box-shadow: var(--shadow);
		transition: box-shadow 0.2s;
		text-decoration: none;
		color: inherit;
	}

	.account-card:hover {
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
	}

	.archived {
		opacity: 0.6;
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	.account-name {
		font-weight: 600;
		font-size: 1rem;
	}

	.institution {
		font-size: 0.8rem;
		color: var(--muted-light);
		margin-top: 0.15rem;
	}

	.account-type {
		font-size: 0.75rem;
		background: #f0f0f0;
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		color: var(--muted);
		white-space: nowrap;
	}

	.card-body {
		display: flex;
		flex-direction: column;
	}

	.balance-label {
		font-size: 0.75rem;
		color: var(--muted-light);
	}

	.balance-value {
		font-size: 1.5rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.balance-value.debt {
		color: var(--neg);
	}

	.since {
		font-size: 0.7rem;
		color: var(--muted-light);
		margin-top: 0.25rem;
	}

	.badge {
		position: absolute;
		top: 0.75rem;
		right: 0.75rem;
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		background: var(--divider);
		color: var(--muted);
		padding: 0.1rem 0.4rem;
		border-radius: 4px;
	}
</style>
