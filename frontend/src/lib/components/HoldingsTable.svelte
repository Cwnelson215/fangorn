<script lang="ts">
	import type { Holdings } from '$lib/types';
	import { formatCurrency, formatPercent, formatPrice, formatShares, formatSigned } from '$lib/format';

	// Also renders the household-wide summary, which has no account_id.
	let { holdings }: { holdings: Omit<Holdings, 'account_id'> } = $props();

	function tone(n: number | null | undefined): string {
		if (n == null || Math.abs(n) < 0.005) return '';
		return n > 0 ? 'pos' : 'neg';
	}
</script>

<div class="table-scroll">
	<div class="table">
		<div class="row head">
			<span>Holding</span>
			<span class="right">Shares</span>
			<span class="right">Price</span>
			<span class="right">Value</span>
			<span class="right">Gain</span>
			<span class="right">Weight</span>
		</div>

		{#each holdings.positions as p (p.symbol)}
			<div class="row">
				<span class="holding">
					<span class="symbol">{p.symbol}</span>
					<span class="sub">{p.name ?? ''}</span>
				</span>
				<span class="right num">
					{formatShares(p.shares)}
					<span class="sub">avg {formatPrice(p.avg_cost)}</span>
				</span>
				<span class="right num">
					{formatPrice(p.price)}
					{#if p.day_change_pct != null}
						<span class="sub {tone(p.day_change_pct)}">{formatPercent(p.day_change_pct, true)} today</span>
					{:else if p.price_time == null}
						<span class="sub" title={p.fetch_error ?? undefined}>your trade price</span>
					{/if}
				</span>
				<span class="right num">
					{formatCurrency(p.market_value)}
					{#if p.day_change != null}
						<span class="sub {tone(p.day_change)}">{formatSigned(p.day_change)}</span>
					{/if}
				</span>
				<span class="right num {tone(p.unrealized_gain)}">
					{formatSigned(p.unrealized_gain)}
					{#if p.unrealized_gain_pct != null}
						<span class="sub {tone(p.unrealized_gain)}">{formatPercent(p.unrealized_gain_pct, true)}</span>
					{/if}
				</span>
				<span class="right num">{formatPercent(p.weight)}</span>
			</div>
		{/each}

		<div class="row summary">
			<span class="holding"><span class="symbol">Cash</span></span>
			<span></span>
			<span></span>
			<span class="right num" class:neg={holdings.cash < 0}>{formatCurrency(holdings.cash)}</span>
			<span></span>
			<span></span>
		</div>
		<div class="row summary total">
			<span class="holding"><span class="symbol">Total</span></span>
			<span></span>
			<span></span>
			<span class="right num">
				{formatCurrency(holdings.total_value)}
				{#if holdings.positions.length > 0}
					<span class="sub {tone(holdings.day_change)}">{formatSigned(holdings.day_change)} today</span>
				{/if}
			</span>
			<span class="right num {tone(holdings.unrealized_gain)}">
				{formatSigned(holdings.unrealized_gain)}
				{#if holdings.cost_basis > 0}
					<span class="sub {tone(holdings.unrealized_gain)}">
						{formatPercent(holdings.unrealized_gain / holdings.cost_basis, true)}
					</span>
				{/if}
			</span>
			<span></span>
		</div>
	</div>
</div>

<style>
	.table-scroll {
		overflow-x: auto;
	}

	.table {
		min-width: 680px;
	}

	.row {
		display: grid;
		grid-template-columns: minmax(180px, 1fr) 110px 120px 130px 120px 70px;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid #f0f0f0;
		align-items: start;
		font-size: 0.9rem;
	}

	.head {
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

	.summary {
		background: none;
	}

	.total {
		border-bottom: none;
		border-top: 2px solid var(--divider);
		font-weight: 700;
	}

	.holding {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.symbol {
		font-weight: 600;
	}

	.num {
		display: flex;
		flex-direction: column;
		font-variant-numeric: tabular-nums;
		font-weight: 600;
	}

	.sub {
		font-size: 0.75rem;
		font-weight: 400;
		color: var(--muted-light);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sub.pos {
		color: var(--pos);
	}

	.sub.neg {
		color: var(--neg);
	}

	.right {
		text-align: right;
		align-items: flex-end;
	}
</style>
