<script lang="ts">
	import type { HoldingPosition, Holdings } from '$lib/types';
	import { formatCurrency, formatPercent, formatPrice, formatShares, formatSigned } from '$lib/format';

	// Also renders the household-wide summary, which has no account_id.
	let { holdings }: { holdings: Omit<Holdings, 'account_id'> } = $props();

	// Which merged positions are opened up to their accounts, by symbol.
	let open = $state<Record<string, boolean>>({});

	function tone(n: number | null | undefined): string {
		if (n == null || Math.abs(n) < 0.005) return '';
		return n > 0 ? 'pos' : 'neg';
	}
</script>

{#snippet position(p: HoldingPosition, accounts: number)}
	<span class="holding">
		<span class="symbol">
			{#if accounts > 0}
				<svg class="chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="M9 6l6 6-6 6" /></svg>
			{/if}
			{p.symbol}
		</span>
		<span class="sub">
			{#if accounts > 0}in {accounts} accounts{#if p.name}{` · ${p.name}`}{/if}{:else}{p.name ?? ''}{/if}
		</span>
	</span>
	<span class="right num shares">
		{formatShares(p.shares)}
		<span class="sub">avg {formatPrice(p.avg_cost)}</span>
	</span>
	<span class="right num price">
		{formatPrice(p.price)}
		{#if p.day_change_pct != null}
			<span class="sub {tone(p.day_change_pct)}">{formatPercent(p.day_change_pct, true)} today</span>
		{:else if p.price_time == null}
			<span class="sub" title={p.fetch_error ?? undefined}>your trade price</span>
		{/if}
	</span>
	<span class="right num value">
		{formatCurrency(p.market_value)}
		{#if p.day_change != null}
			<span class="sub {tone(p.day_change)}">{formatSigned(p.day_change)}</span>
		{/if}
	</span>
	<span class="right num gain {tone(p.unrealized_gain)}">
		{formatSigned(p.unrealized_gain)}
		{#if p.unrealized_gain_pct != null}
			<span class="sub {tone(p.unrealized_gain)}">{formatPercent(p.unrealized_gain_pct, true)}</span>
		{/if}
	</span>
	<span class="right num weight">{formatPercent(p.weight)}</span>
	<!-- Phone only: the shares/price/weight columns folded into one line. -->
	<span class="terms">
		{formatShares(p.shares)} sh @ {formatPrice(p.price)} · {formatPercent(p.weight)}
	</span>
{/snippet}

<div class="table-scroll">
	<div class="table">
		<div class="row head table-head">
			<span>Holding</span>
			<span class="right">Shares</span>
			<span class="right">Price</span>
			<span class="right">Value</span>
			<span class="right">Gain</span>
			<span class="right">Weight</span>
		</div>

		{#each holdings.positions as p (p.symbol)}
			{@const parts = p.accounts && p.accounts.length > 1 ? p.accounts : null}
			{#if parts}
				<!-- Held in more than one account: the row opens to show each one's part. -->
				<button
					type="button"
					class="row toggle"
					aria-expanded={!!open[p.symbol]}
					onclick={() => (open[p.symbol] = !open[p.symbol])}
				>
					{@render position(p, parts.length)}
				</button>
				{#if open[p.symbol]}
					{#each parts as part (part.account_id)}
						<div class="row part">
							<span class="holding">
								<span class="account">{part.account_name}</span>
							</span>
							<span class="right num shares">
								{formatShares(part.shares)}
								<span class="sub">avg {formatPrice(part.avg_cost)}</span>
							</span>
							<span class="price"></span>
							<span class="right num value">
								{formatCurrency(part.market_value)}
								{#if part.day_change != null}
									<span class="sub {tone(part.day_change)}">{formatSigned(part.day_change)}</span>
								{/if}
							</span>
							<span class="right num gain {tone(part.unrealized_gain)}">
								{formatSigned(part.unrealized_gain)}
								{#if part.unrealized_gain_pct != null}
									<span class="sub {tone(part.unrealized_gain)}">
										{formatPercent(part.unrealized_gain_pct, true)}
									</span>
								{/if}
							</span>
							<span class="right num weight">{formatPercent(part.weight)}</span>
							<span class="terms">{formatShares(part.shares)} sh · {formatPercent(part.weight)}</span>
						</div>
					{/each}
				{/if}
			{:else}
				<div class="row">{@render position(p, 0)}</div>
			{/if}
		{/each}

		<div class="row summary">
			<span class="holding"><span class="symbol">Cash</span></span>
			<span class="shares"></span>
			<span class="price"></span>
			<span class="right num value" class:neg={holdings.cash < 0}>{formatCurrency(holdings.cash)}</span>
			<span class="gain"></span>
			<span class="weight"></span>
		</div>
		<div class="row summary total">
			<span class="holding"><span class="symbol">Total</span></span>
			<span class="shares"></span>
			<span class="price"></span>
			<span class="right num value">
				{formatCurrency(holdings.total_value)}
				{#if holdings.positions.length > 0}
					<span class="sub {tone(holdings.day_change)}">{formatSigned(holdings.day_change)} today</span>
				{/if}
			</span>
			<span class="right num gain {tone(holdings.unrealized_gain)}">
				{formatSigned(holdings.unrealized_gain)}
				{#if holdings.cost_basis > 0}
					<span class="sub {tone(holdings.unrealized_gain)}">
						{formatPercent(holdings.unrealized_gain / holdings.cost_basis, true)}
					</span>
				{/if}
			</span>
			<span class="weight"></span>
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
		border-bottom: 1px solid var(--line);
		align-items: start;
		font-size: 0.9rem;
	}

	/* A position held in several accounts is a button that opens its parts. */
	.toggle {
		width: 100%;
		margin: 0;
		background: none;
		border: none;
		border-bottom: 1px solid var(--line);
		color: inherit;
		font: inherit;
		font-size: 0.9rem;
		text-align: left;
		cursor: pointer;
	}

	.toggle:hover {
		background: var(--surface-2);
	}

	.toggle[aria-expanded='true'] {
		border-bottom-color: transparent;
	}

	.chevron {
		width: 14px;
		height: 14px;
		margin-left: -2px;
		vertical-align: -2px;
		fill: none;
		stroke: currentColor;
		stroke-width: 2.5;
		stroke-linecap: round;
		stroke-linejoin: round;
		color: var(--muted-light);
		transition: transform 0.15s;
	}

	.toggle[aria-expanded='true'] .chevron {
		transform: rotate(90deg);
	}

	/* An account's part, tucked under its position. */
	.part {
		background: var(--surface-2);
		font-size: 0.85rem;
	}

	.part .num {
		font-weight: 500;
	}

	.account {
		padding-left: 1.25rem;
		color: var(--muted-light);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
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

	.terms {
		display: none;
	}

	/* Phone: what it is and what it's worth on top; how much of it and how
	   it's done underneath. */
	@media (max-width: 639px) {
		.row {
			grid-template-columns: minmax(0, 1fr) auto auto;
			grid-template-areas:
				'holding holding value'
				'terms terms gain';
			column-gap: 0.75rem;
			row-gap: 0.125rem;
			padding: 0.625rem 0.25rem;
		}

		.holding {
			grid-area: holding;
		}

		.value {
			grid-area: value;
		}

		.gain {
			grid-area: gain;
			flex-direction: row;
			gap: 0.375rem;
			align-items: baseline;
			font-size: 0.8125rem;
		}

		.shares,
		.price,
		.weight {
			display: none;
		}

		.terms {
			display: block;
			grid-area: terms;
			align-self: end;
			font-size: 0.75rem;
			color: var(--muted-light);
			font-variant-numeric: tabular-nums;
			white-space: nowrap;
			overflow: hidden;
			text-overflow: ellipsis;
		}

		.value .sub {
			font-size: 0.75rem;
		}
	}
</style>
