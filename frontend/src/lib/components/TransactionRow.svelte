<script lang="ts">
	import type { Transaction } from '$lib/types';
	import { formatCurrency, formatDateShort, formatSigned } from '$lib/format';

	let {
		transaction,
		showAccount = true,
		showRunningBalance = false,
		onedit
	}: {
		transaction: Transaction;
		showAccount?: boolean;
		showRunningBalance?: boolean;
		onedit?: (transaction: Transaction) => void;
	} = $props();

	let isTransfer = $derived(transaction.kind === 'transfer');
	let isTrade = $derived(transaction.kind === 'trade');
	let isRefund = $derived(transaction.kind === 'refund');
	// Transfers and trades move money without earning or spending it, so neither
	// is coloured like income or an expense.
	let isNeutral = $derived(isTransfer || isTrade);
	let isIncome = $derived(transaction.amount > 0 && !isNeutral);
	let isExpense = $derived(transaction.amount < 0 && !isNeutral);
</script>

{#snippet cells()}
	<span class="date">{formatDateShort(transaction.date)}</span>

	<span class="desc">
		<span class="name">{transaction.description}</span>
		<span class="subtext">
			{#if transaction.merchant}{transaction.merchant}{/if}
			{#if showAccount && transaction.account_name}
				{#if transaction.merchant}·{/if}
				{transaction.account_name}
			{/if}
			{#if transaction.source === 'recurring'}· auto{/if}
		</span>
	</span>

	<span class="tags">
		{#if isTransfer}
			<span class="tag transfer">Transfer</span>
		{:else if isTrade}
			<span class="tag trade">Trade</span>
		{:else if isRefund}
			<!-- A refund reads as money in, so the tag has to say which spending it
			     came back from or the row looks like a windfall. -->
			<span class="tag refund">Refund{#if transaction.category_name}: {transaction.category_name}{/if}</span>
		{:else if transaction.category_name}
			<span class="tag">{transaction.category_name}</span>
		{/if}
	</span>

	<span class="amount" class:pos={isIncome} class:neg={isExpense}>
		{formatSigned(transaction.amount)}
	</span>

	{#if showRunningBalance}
		<span class="running">
			{transaction.running_balance !== undefined ? formatCurrency(transaction.running_balance) : ''}
		</span>
	{/if}
{/snippet}

<!-- An editable row is a real <button>, not a div wearing role="button". That
     gets keyboard activation, focus, and screen-reader semantics for free. -->
{#if onedit}
	<button class="row clickable" class:with-balance={showRunningBalance} onclick={() => onedit(transaction)}>
		{@render cells()}
	</button>
{:else}
	<div class="row" class:with-balance={showRunningBalance}>
		{@render cells()}
	</div>
{/if}

<style>
	.row {
		display: grid;
		grid-template-columns: 80px 1fr 150px 120px;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid #f0f0f0;
		align-items: center;
		font-size: 0.9rem;
		text-align: left;
		width: 100%;
		/* Reset the button chrome so both branches render identically. */
		background: none;
		border-left: none;
		border-right: none;
		border-top: none;
		font-family: inherit;
		color: inherit;
	}

	.row.with-balance {
		grid-template-columns: 80px 1fr 150px 120px 120px;
	}

	.row:last-child {
		border-bottom: none;
	}

	.clickable {
		cursor: pointer;
	}

	.clickable:hover,
	.clickable:focus-visible {
		background: #fafafa;
		outline: none;
	}

	.date {
		color: var(--muted-light);
		font-size: 0.85rem;
	}

	.desc {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.name {
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.subtext {
		font-size: 0.75rem;
		color: var(--muted-light);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.subtext:empty {
		display: none;
	}

	.tag {
		display: inline-block;
		font-size: 0.75rem;
		background: #e8f5e9;
		color: #2e7d32;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
	}

	.tag.transfer {
		background: #e3f2fd;
		color: #1565c0;
	}

	.tag.trade {
		background: #f3e8ff;
		color: #7e22ce;
	}

	.tag.refund {
		background: #fff4e5;
		color: #b26a00;
	}

	.amount,
	.running {
		text-align: right;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.running {
		font-weight: 500;
		color: var(--muted);
	}

	/* Transfers are intentionally neutral: money moving between your own accounts
	   is neither income nor spending. */
	.amount {
		color: var(--ink);
	}
</style>
