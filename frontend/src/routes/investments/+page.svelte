<script lang="ts">
	import { onMount } from 'svelte';
	import { getInvestments, getInvestmentsHistory } from '$lib/api';
	import type { InvestmentsSummary, Slice } from '$lib/types';
	import { formatCurrency, formatMarketTime, formatPercent, formatSigned } from '$lib/format';
	import { startPolling } from '$lib/poll';
	import HoldingsTable from '$lib/components/HoldingsTable.svelte';
	import DonutChart from '$lib/components/DonutChart.svelte';
	import ValueHistoryCard from '$lib/components/ValueHistoryCard.svelte';

	// Same cadence as the account page: the server refreshes prices at most once
	// a minute during market hours.
	const POLL_MS = 60_000;

	let summary = $state<InvestmentsSummary | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let refreshError = $state<string | null>(null);

	onMount(() => {
		load();
		return startPolling(refresh, POLL_MS);
	});

	async function load() {
		loading = true;
		error = null;
		try {
			summary = await getInvestments();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load your investments';
		} finally {
			loading = false;
		}
	}

	async function refresh() {
		if (!summary) return;
		try {
			summary = await getInvestments();
			refreshError = null;
		} catch (e) {
			refreshError = e instanceof Error ? e.message : 'Could not refresh prices';
		}
	}

	function tone(n: number | null | undefined): string {
		if (n == null || Math.abs(n) < 0.005) return '';
		return n > 0 ? 'pos' : 'neg';
	}

	// Holdings only, so the percentages match the table's Weight column.
	let slices = $derived<Slice[]>(
		(summary?.positions ?? [])
			.map((p) => ({ label: p.symbol, value: p.market_value }))
			.sort((a, b) => b.value - a.value)
	);

	let hasFunds = $derived(summary?.positions.some((p) => p.quote_type === 'MUTUALFUND') ?? false);
</script>

<div class="page">
	<div class="page-header">
		<h1>Investments</h1>
		{#if summary?.as_of}
			<span class="muted">Prices as of {formatMarketTime(summary.as_of)}</span>
		{/if}
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if summary && summary.accounts.length === 0}
		<div class="card empty">
			<h2>No investment accounts yet</h2>
			<p class="muted">
				Add a brokerage, IRA or 401(k) as an <strong>Investment</strong> account, then log what it
				holds from its page. Prices update on their own.
			</p>
			<a class="cta" href="/accounts">Add an account</a>
		</div>
	{:else if summary}
		<div class="stats">
			<div class="card stat">
				<span class="stat-label">Total Value</span>
				<span class="stat-value">{formatCurrency(summary.total_value)}</span>
				<span class="muted">
					{formatCurrency(summary.holdings_value)} invested · {formatCurrency(summary.cash)} cash
				</span>
			</div>
			<div class="card stat">
				<span class="stat-label">Today</span>
				<span class="stat-value {tone(summary.day_change)}">{formatSigned(summary.day_change)}</span>
				<span class="muted">
					{summary.day_change_pct != null ? formatPercent(summary.day_change_pct, true) : 'no change yet'}
				</span>
			</div>
			<div class="card stat">
				<span class="stat-label">Unrealized Gain</span>
				<span class="stat-value {tone(summary.unrealized_gain)}">
					{formatSigned(summary.unrealized_gain)}
				</span>
				<span class="muted">
					{#if summary.cost_basis > 0}
						{formatPercent(summary.unrealized_gain / summary.cost_basis, true)} on
						{formatCurrency(summary.cost_basis)} cost
					{:else}
						nothing held
					{/if}
				</span>
			</div>
			<div class="card stat">
				<span class="stat-label">Realized Gain</span>
				<span class="stat-value {tone(summary.realized_gain)}">
					{formatSigned(summary.realized_gain)}
				</span>
				<span class="muted">from shares sold</span>
			</div>
		</div>

		<ValueHistoryCard id="investments-value" load={getInvestmentsHistory} />

		<div class="card">
			<h2>Holdings</h2>
			{#if summary.positions.length === 0}
				<p class="muted small">Nothing held yet. Log trades from an investment account's page.</p>
			{:else}
				<HoldingsTable holdings={summary} />
			{/if}
			<p class="muted small note">
				{#if hasFunds}Mutual funds price once a day, after the market closes.{/if}
				{#if summary.seeded}Some holdings are valued at the price you logged until a quote comes in.{/if}
				{#if refreshError}<span class="error-text">{refreshError}</span>{/if}
			</p>
		</div>

		<div class="grid-2">
			<div class="card">
				<h2>Allocation</h2>
				{#if slices.length > 0}
					<DonutChart
						{slices}
						centerLabel="Invested"
						legendValue={(s, total) => formatPercent(s.value / total)}
					/>
				{:else}
					<p class="muted small">Nothing to show yet.</p>
				{/if}
			</div>

			<div class="card">
				<h2>Accounts</h2>
				<div class="account-list">
					{#each summary.accounts as account (account.id)}
						<a class="account-line" href="/accounts/{account.id}">
							<span class="account-name">
								{account.name}
								{#if account.institution_name}
									<span class="muted">{account.institution_name}</span>
								{/if}
							</span>
							<span class="account-value">
								{formatCurrency(account.total_value)}
								{#if account.holdings_value > 0}
									<span class="muted {tone(account.day_change)}">
										{formatSigned(account.day_change)} today
									</span>
								{/if}
							</span>
						</a>
					{/each}
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	h2 {
		font-size: 1rem;
		margin-bottom: 1rem;
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 1rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}

	.stat-label {
		font-size: 0.8125rem;
		color: var(--muted);
		font-weight: 500;
	}

	.stat-value {
		font-size: 1.75rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.grid-2 {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(340px, 100%), 1fr));
		gap: 1.5rem;
		align-items: start;
	}

	.note {
		margin-top: 0.75rem;
	}

	@media (max-width: 639px) {
		/* Total value gets the full width; today, unrealized and realized share a row. */
		.stats {
			grid-template-columns: repeat(3, minmax(0, 1fr));
			gap: 0.5rem;
		}

		.stats .stat:first-child {
			grid-column: 1 / -1;
		}

		.stats .stat:not(:first-child) {
			padding: 0.75rem;
		}

		.stats .stat:not(:first-child) .stat-value {
			font-size: 1rem;
		}

		.stats .stat:not(:first-child) .stat-label,
		.stats .stat:not(:first-child) .muted {
			font-size: 0.6875rem;
		}
	}

	.small {
		font-size: 0.875rem;
	}

	.account-list {
		display: flex;
		flex-direction: column;
	}

	.account-line {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.625rem 0;
		border-bottom: 1px solid var(--divider);
		text-decoration: none;
		color: inherit;
	}

	.account-line:last-child {
		border-bottom: none;
	}

	.account-line:hover .account-name {
		color: var(--accent-hover);
	}

	.account-name,
	.account-value {
		display: flex;
		flex-direction: column;
		line-height: 1.3;
	}

	.account-value {
		align-items: flex-end;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.account-name .muted,
	.account-value .muted {
		font-size: 0.75rem;
		font-weight: 400;
	}

	.empty h2 {
		margin-bottom: 0.5rem;
	}

	.empty .muted {
		max-width: 30rem;
		margin: 0 auto 1.5rem;
	}

	.cta {
		display: inline-block;
		background: var(--accent);
		color: var(--ink);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
	}

	.cta:hover {
		background: var(--accent-hover);
	}
</style>
