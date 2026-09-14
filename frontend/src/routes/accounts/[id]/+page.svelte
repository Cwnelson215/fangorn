<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getAccount, getAccountValueHistory, getHoldings, getTrades } from '$lib/api';
	import type { AccountDetail, Holdings, Trade } from '$lib/types';
	import { ACCOUNT_TYPE_LABELS, TRADE_SIDE_LABELS } from '$lib/types';
	import {
		formatCurrency,
		formatDate,
		formatDateShort,
		formatMarketTime,
		formatPercent,
		formatPrice,
		formatShares,
		formatSigned
	} from '$lib/format';
	import { startPolling } from '$lib/poll';
	import TransactionRow from '$lib/components/TransactionRow.svelte';
	import HoldingsTable from '$lib/components/HoldingsTable.svelte';
	import TradeModal from '$lib/components/TradeModal.svelte';
	import ValueHistoryCard from '$lib/components/ValueHistoryCard.svelte';
	import Button from '$lib/components/Button.svelte';

	// Prices are refreshed server-side at most once a minute during market hours,
	// so polling faster than that would only re-read the same numbers.
	const HOLDINGS_POLL_MS = 60_000;

	let detail = $state<AccountDetail | null>(null);
	let holdings = $state<Holdings | null>(null);
	let trades = $state<Trade[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let holdingsError = $state<string | null>(null);

	let tradeModalOpen = $state(false);
	let editingTrade = $state<Trade | null>(null);

	let accountId = $derived(Number(page.params.id));
	let isInvestment = $derived(detail?.account.type === 'investment');

	onMount(() => {
		load();
		return startPolling(refreshHoldings, HOLDINGS_POLL_MS);
	});

	async function load() {
		loading = true;
		error = null;
		try {
			detail = await getAccount(accountId);
			if (detail.account.type === 'investment') {
				[holdings, trades] = await Promise.all([getHoldings(accountId), getTrades(accountId)]);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load this account';
		} finally {
			loading = false;
		}
	}

	// A poll only replaces the holdings; the register and trade log can't change
	// without someone saving, which reloads everything.
	async function refreshHoldings() {
		if (!isInvestment) return;
		try {
			holdings = await getHoldings(accountId);
			holdingsError = null;
		} catch (e) {
			holdingsError = e instanceof Error ? e.message : 'Could not refresh prices';
		}
	}

	function openNewTrade() {
		editingTrade = null;
		tradeModalOpen = true;
	}

	function openTrade(trade: Trade) {
		editingTrade = trade;
		tradeModalOpen = true;
	}

	let isLiability = $derived(detail?.account.class === 'liability');
	let hasFunds = $derived(holdings?.positions.some((p) => p.quote_type === 'MUTUALFUND') ?? false);
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
				{#if isInvestment && holdings}
					<span class="muted">Total value</span>
					<span class="balance">{formatCurrency(holdings.total_value)}</span>
					{#if holdings.positions.length > 0}
						<span
							class="day-change"
							class:pos={holdings.day_change > 0.004}
							class:neg={holdings.day_change < -0.004}
						>
							{formatSigned(holdings.day_change)}
							{#if holdings.day_change_pct != null}({formatPercent(holdings.day_change_pct, true)}){/if}
							today
						</span>
					{/if}
					<span class="muted small">
						Cash {formatCurrency(holdings.cash)} · Invested {formatCurrency(holdings.holdings_value)}
					</span>
				{:else}
					<span class="muted">{isLiability ? 'Owed' : 'Balance'}</span>
					<span class="balance" class:neg={isLiability && account.balance !== 0}>
						{formatCurrency(isLiability ? Math.abs(account.balance) : account.balance)}
					</span>
					<span class="muted small">
						Started at {formatCurrency(Math.abs(account.starting_balance))} on
						{formatDate(account.starting_balance_date)}
					</span>
				{/if}
			</div>
		</div>

		{#if isInvestment && holdings}
			<div class="card">
				<div class="card-header">
					<h2>Holdings</h2>
					<Button size="sm" onclick={openNewTrade}>Log trade</Button>
				</div>

				{#if holdings.positions.length === 0 && trades.length === 0}
					<p class="muted small">
						Nothing held yet. Log what this account owns — use <strong>Already owned</strong> for
						shares you had before you started tracking it, and <strong>Buy</strong> for new
						purchases.
					</p>
				{:else}
					<HoldingsTable {holdings} />
				{/if}

				<p class="muted small price-note">
					{#if holdings.as_of}
						Prices as of {formatMarketTime(holdings.as_of)}.
					{/if}
					{#if hasFunds}
						Mutual funds price once a day, after the market closes.
					{/if}
					{#if holdings.seeded}
						Some holdings are valued at the price you logged until a quote comes in.
					{/if}
					{#if holdingsError}
						<span class="error-text">{holdingsError}</span>
					{/if}
				</p>
				{#if holdings.cash < 0}
					<p class="warn small">
						Cash is below zero. If you funded these buys with a transfer, log it from the Transfers
						page.
					</p>
				{/if}
			</div>

			{#if trades.length > 0}
				<!-- load() unmounts the page while it reloads, so this re-fetches after a trade. -->
				<ValueHistoryCard id="account-value" load={(days) => getAccountValueHistory(accountId, days)} />

				<div class="card">
					<h2>Trades</h2>
					<div class="table-scroll">
						<div class="trades">
							<div class="trade-row head">
								<span>Date</span>
								<span>Trade</span>
								<span class="right">Shares</span>
								<span class="right">Price</span>
								<span class="right">Amount</span>
							</div>
							{#each trades as trade (trade.id)}
								<button class="trade-row clickable" onclick={() => openTrade(trade)}>
									<span class="date">{formatDateShort(trade.trade_date)}</span>
									<span class="desc">
										<span class="name">{TRADE_SIDE_LABELS[trade.side]} {trade.symbol}</span>
										<span class="sub">{trade.security_name ?? ''}</span>
									</span>
									<span class="right num">{formatShares(trade.shares)}</span>
									<span class="right num">{formatPrice(trade.price)}</span>
									<span class="right num">{formatCurrency(trade.amount)}</span>
								</button>
							{/each}
						</div>
					</div>
				</div>
			{/if}
		{/if}

		<div class="card">
			<h2>{isInvestment ? 'Cash activity' : 'Register'}</h2>
			{#if detail.transactions.length === 0}
				<p class="muted small">
					{#if isInvestment}
						No cash has moved yet. Transfers in, cash dividends and buys and sells show up here.
					{:else}
						Nothing logged yet. The balance above is still the starting balance you entered.
					{/if}
				</p>
			{:else}
				<div class="table-scroll">
					<div class="table">
						<div class="head">
							<span>Date</span>
							<span>Description</span>
							<span>Category</span>
							<span class="right">Amount</span>
							<span class="right">{isInvestment ? 'Cash' : 'Balance'}</span>
						</div>
						{#each detail.transactions as transaction (transaction.id)}
							<TransactionRow {transaction} showAccount={false} showRunningBalance={true} />
						{/each}
					</div>
				</div>
			{/if}
		</div>

		{#if isInvestment}
			<TradeModal {accountId} trade={editingTrade} bind:open={tradeModalOpen} onsaved={load} />
		{/if}
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

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.card-header h2 {
		margin-bottom: 0;
	}

	.day-change {
		font-size: 0.875rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.price-note {
		margin-top: 1rem;
	}

	.warn {
		margin-top: 0.5rem;
		color: var(--warn);
		font-weight: 600;
	}

	.trades {
		min-width: 560px;
	}

	.trade-row {
		display: grid;
		grid-template-columns: 80px 1fr 110px 110px 120px;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border: none;
		border-bottom: 1px solid #f0f0f0;
		align-items: center;
		width: 100%;
		background: none;
		font: inherit;
		font-size: 0.9rem;
		text-align: left;
		color: inherit;
	}

	.trade-row:last-child {
		border-bottom: none;
	}

	.trade-row.head {
		padding: 0.5rem 1rem;
		background: var(--bg);
		border-radius: var(--radius-sm);
		border-bottom: none;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
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
	}

	.sub {
		font-size: 0.75rem;
		color: var(--muted-light);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.num {
		font-variant-numeric: tabular-nums;
		font-weight: 600;
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
