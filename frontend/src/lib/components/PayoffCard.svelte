<script lang="ts">
	// "When is this paid off, and what does it cost?" for a credit card or a
	// loan. The rate isn't stored on the account, so it's an assumption here like
	// the payment: the user sets both, and an extra-payment slider shows what
	// paying more saves, side by side with the plan as it stands.
	import type { ValuePoint } from '$lib/types';
	import { formatCurrency, formatCurrencyWhole, formatDate, today } from '$lib/format';
	import { addMonths, formatDuration, projectPayoff } from '$lib/projection';
	import { forget, recallValues, rememberValues } from '$lib/remember';
	import { SERIES } from '$lib/chart';
	import ProjectionChart from './ProjectionChart.svelte';
	import SliderField from './SliderField.svelte';
	import Button from './Button.svelte';

	let {
		id,
		kind,
		owed,
		defaultPayment,
		loadHistory
	}: {
		id: string;
		kind: 'credit_card' | 'loan';
		/** What's owed today, positive. */
		owed: number;
		/** What the recurring rules already pay each month. */
		defaultPayment: number;
		loadHistory?: () => Promise<ValuePoint[]>;
	} = $props();

	let isCard = $derived(kind === 'credit_card');

	function defaults() {
		// Without a scheduled payment, start from something that clears it in a
		// reasonable time: about three years on a card, five on a loan.
		const fallback = Math.max(25, Math.ceil(owed / (kind === 'credit_card' ? 30 : 50) / 5) * 5);
		return {
			apr: kind === 'credit_card' ? 22 : 7,
			payment: Math.round(defaultPayment > 0 ? defaultPayment : fallback),
			extra: 0
		};
	}

	// svelte-ignore state_referenced_locally
	const initial = recallValues(`payoff.${id}`, defaults());
	let apr = $state(initial.apr);
	let payment = $state(initial.payment);
	let extra = $state(initial.extra);

	// Only a change is remembered: values left at their defaults keep following
	// the recurring rules they came from.
	$effect(() => {
		const values = { apr, payment, extra };
		const d: Record<string, number | boolean> = defaults();
		if (Object.entries(values).every(([k, v]) => d[k] === v)) forget(`payoff.${id}`);
		else rememberValues(`payoff.${id}`, values);
	});

	function reset() {
		forget(`payoff.${id}`);
		({ apr, payment, extra } = defaults());
	}

	let history = $state<{ date: string; value: number }[]>([]);
	$effect(() => {
		if (!loadHistory) return;
		let stale = false;
		loadHistory()
			.then((pts) => {
				// A liability's balance is negative; the chart shows what's owed.
				if (!stale) history = pts.map((p) => ({ date: p.date, value: -p.value }));
			})
			.catch(() => {
				// The projection stands on its own without the history.
			});
		return () => {
			stale = true;
		};
	});

	let now = today();
	let monthlyInterest = $derived(owed * (apr / 100 / 12));
	let plan = $derived(projectPayoff({ owed, apr: apr / 100, payment }));
	let faster = $derived(extra > 0 ? projectPayoff({ owed, apr: apr / 100, payment: payment + extra }) : null);

	// Both plans on one set of months, the shorter one sitting at zero once
	// it's done.
	let rows = $derived.by(() => {
		const n = Math.max(plan.points.length, faster?.points.length ?? 0);
		return Array.from({ length: n }, (_, m) => ({
			date: addMonths(now, m),
			plan: plan.points[m]?.owed ?? 0,
			faster: faster ? (faster.points[m]?.owed ?? 0) : 0
		}));
	});

	let lines = $derived([
		{ key: 'plan', label: `Paying ${formatCurrencyWhole(payment)}/mo`, color: SERIES.orange },
		...(faster ? [{ key: 'faster', label: `Paying ${formatCurrencyWhole(payment + extra)}/mo`, color: SERIES.green }] : [])
	]);

	let saved = $derived(
		faster && plan.months != null && faster.months != null
			? { interest: plan.totalInterest - faster.totalInterest, months: plan.months - faster.months }
			: null
	);
</script>

<div class="card">
	<div class="head">
		<div>
			<h2>Payoff</h2>
			<p class="muted sub">
				{formatCurrency(owed)} at {apr}% APR, paid {formatCurrencyWhole(payment + extra)} a month{isCard
					? ', with no new charges'
					: ''}.
			</p>
		</div>
		<Button variant="ghost" size="sm" onclick={reset}>Reset</Button>
	</div>

	{#if owed < 0.005}
		<p class="muted">Nothing owed — nothing to pay off.</p>
	{:else}
		{@const best = faster ?? plan}
		<div class="stats">
			<div>
				<span class="label">Paid off</span>
				{#if best.months != null}
					<span class="value">{formatDate(addMonths(now, best.months))}</span>
					<span class="muted small">in {formatDuration(best.months)}</span>
				{:else}
					<span class="value neg">Never</span>
					<span class="muted small">at this payment</span>
				{/if}
			</div>
			<div>
				<span class="label">Interest</span>
				<span class="value">{formatCurrencyWhole(best.totalInterest)}</span>
				<span class="muted small">
					{best.months != null ? 'over the payoff' : 'over the next 10 years'}
				</span>
			</div>
			<div>
				<span class="label">{saved ? 'Paying more saves' : 'Interest this month'}</span>
				{#if saved}
					<span class="value pos">{formatCurrencyWhole(saved.interest)}</span>
					<span class="muted small">and {formatDuration(saved.months)} sooner</span>
				{:else}
					<span class="value">≈ {formatCurrency(monthlyInterest)}</span>
					<span class="muted small">of each payment goes to interest now</span>
				{/if}
			</div>
		</div>

		{#if payment + extra <= monthlyInterest}
			<p class="warn small">
				{formatCurrency(payment + extra)} a month doesn't cover the ≈{formatCurrency(monthlyInterest)} of
				interest, so the balance never comes down.
			</p>
		{/if}

		<ProjectionChart {history} historyLabel="Owed so far" {rows} {lines} />
	{/if}

	<div class="controls">
		<SliderField
			label="APR"
			id="{id}-apr"
			bind:value={apr}
			min={0}
			max={isCard ? 36 : 15}
			step={isCard ? 0.25 : 0.125}
			suffix="%"
			hint="From the {isCard ? 'statement' : 'loan terms'} — not stored on the account"
		/>
		<SliderField
			label="Monthly payment"
			id="{id}-payment"
			bind:value={payment}
			min={0}
			max={Math.max(500, Math.ceil(owed / 12 / 100) * 100, payment)}
			step={5}
			prefix="$"
			hint={defaultPayment > 0 ? `Your recurring payments add ${formatCurrencyWhole(defaultPayment)}` : undefined}
		/>
		<SliderField
			label="Extra each month"
			id="{id}-extra"
			bind:value={extra}
			min={0}
			max={Math.max(500, Math.ceil(payment / 100) * 100)}
			step={10}
			prefix="$"
			hint="See what paying more saves"
		/>
	</div>
</div>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	h2 {
		font-size: 1rem;
	}

	.sub {
		font-size: 0.8125rem;
		margin: 0.25rem 0 0;
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.stats > div {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.label {
		font-size: 0.8125rem;
		color: var(--muted);
		font-weight: 500;
	}

	.value {
		font-size: 1.375rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.value.pos {
		color: var(--pos);
	}

	.value.neg {
		color: var(--neg);
	}

	.small {
		font-size: 0.75rem;
	}

	.warn {
		color: var(--warn);
		font-weight: 600;
		margin: 0 0 0.75rem;
	}

	.controls {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem 1.5rem;
		margin-top: 1.25rem;
		padding-top: 1rem;
		border-top: 1px solid var(--divider);
	}

	@media (max-width: 639px) {
		.stats {
			grid-template-columns: 1fr 1fr;
		}

		.stats > div:first-child {
			grid-column: 1 / -1;
		}

		.controls {
			grid-template-columns: 1fr;
		}
	}
</style>
