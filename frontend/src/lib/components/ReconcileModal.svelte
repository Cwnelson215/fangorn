<script lang="ts">
	// Check an investment account against the brokerage: type in what the
	// statement shows and see which line is off and why (lib/reconcile.ts).
	// Nothing is saved or changed — it only points at what to fix.
	import type { Holdings } from '$lib/types';
	import { reconcile, type StatementLine } from '$lib/reconcile';
	import { formatCurrency, formatShares, formatSigned } from '$lib/format';
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';

	let {
		open = $bindable(false),
		holdings,
		cashFund = null
	}: { open?: boolean; holdings: Holdings; cashFund?: string | null } = $props();

	// What has been typed, kept as text so a blank field stays "not entered".
	let cash = $state('');
	let shares = $state<Record<string, string>>({});
	let values = $state<Record<string, string>>({});

	function num(text: string | undefined): number | null {
		if (text == null || String(text).trim() === '') return null;
		const n = Number(text);
		return Number.isFinite(n) ? n : null;
	}

	let result = $derived(
		reconcile(holdings, {
			cash: num(cash),
			lines: Object.fromEntries(
				holdings.positions.map((p): [string, StatementLine] => [
					p.symbol,
					{ shares: num(shares[p.symbol]), value: num(values[p.symbol]) }
				])
			)
		})
	);

	function clear() {
		cash = '';
		shares = {};
		values = {};
	}
</script>

<Modal bind:open title="Reconcile with the statement">
	<p class="muted note">
		Enter what the brokerage shows. Leave a line blank to skip it. Prices keep moving while the
		market is open, so a few cents on a position is expected then.
	</p>

	<div class="lines">
		<div class="line head">
			<span>Holding</span>
			<span class="right">Shares</span>
			<span class="right">Value</span>
		</div>
		{#each holdings.positions as p, i (p.symbol)}
			{@const check = result.positions[i]}
			<div class="line">
				<span class="what">
					<span class="symbol">{p.symbol}</span>
					<span class="muted sub">{formatShares(p.shares)} sh · {formatCurrency(p.market_value)}</span>
				</span>
				<input
					type="number"
					inputmode="decimal"
					step="any"
					min="0"
					placeholder={formatShares(p.shares)}
					aria-label="{p.symbol} shares on the statement"
					bind:value={shares[p.symbol]}
				/>
				<input
					type="number"
					inputmode="decimal"
					step="0.01"
					placeholder={p.market_value.toFixed(2)}
					aria-label="{p.symbol} value on the statement"
					bind:value={values[p.symbol]}
				/>
				{#if check.verdict === 'shares'}
					<span class="verdict off">
						{formatShares(Math.abs(check.shareDiff))}
						{check.shareDiff > 0 ? 'more' : 'fewer'} shares on the statement ({formatSigned(
							check.fromShares
						)}) — a trade or reinvested dividend is missing or mistyped.
						{#if Math.abs(check.fromPrice) >= 0.01}
							Price accounts for {formatSigned(check.fromPrice)} more.
						{/if}
					</span>
				{:else if check.verdict === 'price'}
					<span class="verdict muted">
						{formatSigned(check.valueDiff)} — same shares, so only the price differs. It settles
						with the next quote or at the close.
					</span>
				{:else if check.verdict === 'match'}
					<span class="verdict ok">Matches</span>
				{/if}
			</div>
		{/each}
		<div class="line">
			<span class="what">
				<span class="symbol">Cash</span>
				<span class="muted sub">
					{#if cashFund}{cashFund} on the statement · {/if}{formatCurrency(holdings.cash)}
				</span>
			</span>
			<span></span>
			<input
				type="number"
				inputmode="decimal"
				step="0.01"
				placeholder={holdings.cash.toFixed(2)}
				aria-label="Cash on the statement"
				bind:value={cash}
			/>
			{#if result.cashDiff != null}
				{#if Math.abs(result.cashDiff) >= 0.01}
					<span class="verdict off">
						{formatSigned(result.cashDiff)} — a dividend, fee or deposit is missing, or a trade's
						dollar amount is off. Compare the statement's activity with this account's.
					</span>
				{:else}
					<span class="verdict ok">Matches</span>
				{/if}
			{/if}
		</div>
	</div>

	{#if result.checked > 0}
		<div class="summary" class:ok={result.matches}>
			{#if result.matches}
				Everything entered matches.
			{:else}
				<strong>The statement is {formatSigned(result.totalDiff)} against Fangorn</strong> on what
				you entered:
				{formatSigned(result.fromCash)} cash, {formatSigned(result.fromShares)} share counts,
				{formatSigned(result.fromPrice)} prices.
			{/if}
		</div>
	{/if}

	<div class="form-actions">
		<Button variant="ghost" onclick={clear}>Clear</Button>
		<Button onclick={() => (open = false)}>Done</Button>
	</div>
</Modal>

<style>
	.note {
		font-size: 0.8125rem;
		margin: 0 0 0.75rem;
	}

	.line {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 6.5rem 7rem;
		gap: 0.5rem;
		align-items: center;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--divider);
	}

	.line.head {
		font-size: 0.75rem;
		color: var(--muted);
		font-weight: 600;
		padding-top: 0;
	}

	.right {
		text-align: right;
	}

	.what {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.symbol {
		font-weight: 600;
	}

	.sub {
		font-size: 0.75rem;
	}

	.line input {
		width: 100%;
		min-width: 0;
		text-align: right;
		font-variant-numeric: tabular-nums;
	}

	.verdict {
		grid-column: 1 / -1;
		font-size: 0.8125rem;
	}

	.verdict.off {
		color: var(--neg);
	}

	.verdict.ok {
		color: var(--pos);
	}

	.summary {
		margin-top: 0.75rem;
		padding: 0.625rem 0.75rem;
		border-radius: 8px;
		background: var(--bg);
		font-size: 0.875rem;
	}

	.summary.ok {
		color: var(--pos);
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	@media (max-width: 639px) {
		.line {
			grid-template-columns: minmax(0, 1fr) 5rem 5.75rem;
		}
	}
</style>
