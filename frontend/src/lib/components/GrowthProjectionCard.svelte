<script lang="ts">
	// "Where is this headed?" for an account that grows by itself: an investment
	// or retirement account (a return that varies, so a range around it) or a
	// high-yield savings account (a known APY). The history is real; everything
	// after today is the assumptions below it, which the user drags around and
	// this device remembers per account.
	import type { ValuePoint } from '$lib/types';
	import { formatCurrency, formatCurrencyWhole, formatDate, today } from '$lib/format';
	import { addMonths, formatDuration, projectGrowth } from '$lib/projection';
	import { forget, recallValues, rememberValues } from '$lib/remember';
	import { SERIES } from '$lib/chart';
	import ProjectionChart from './ProjectionChart.svelte';
	import SliderField from './SliderField.svelte';
	import Button from './Button.svelte';

	type Kind = 'investment' | 'retirement' | 'savings';

	let {
		id,
		kind,
		start,
		defaultMonthly,
		defaultRate,
		loadHistory
	}: {
		id: string;
		kind: Kind;
		/** Today's value. */
		start: number;
		/** What the recurring rules already put in each month. */
		defaultMonthly: number;
		/** Percent: the account's APY for savings, or a long-run market return. */
		defaultRate?: number;
		loadHistory?: () => Promise<ValuePoint[]>;
	} = $props();

	const INFLATION = 0.025;
	// How far either side of the expected return the shaded range reaches.
	const SPREAD = 2;
	const MILESTONES = [1e3, 5e3, 1e4, 2.5e4, 5e4, 1e5, 2.5e5, 5e5, 1e6, 2e6, 5e6, 1e7];

	let isSavings = $derived(kind === 'savings');

	function defaults() {
		return {
			monthly: Math.round(defaultMonthly),
			rate: defaultRate ?? (kind === 'savings' ? 4 : 7),
			years: kind === 'retirement' ? 25 : kind === 'savings' ? 5 : 10,
			real: false
		};
	}

	// svelte-ignore state_referenced_locally
	const initial = recallValues(`projection.${id}`, defaults());
	let monthly = $state(initial.monthly);
	let rate = $state(initial.rate);
	let years = $state(initial.years);
	let real = $state(initial.real);

	// Only a change is remembered: values left at their defaults keep following
	// the recurring rules they came from.
	$effect(() => {
		const values = { monthly, rate, years, real };
		const d: Record<string, number | boolean> = defaults();
		if (Object.entries(values).every(([k, v]) => d[k] === v)) forget(`projection.${id}`);
		else rememberValues(`projection.${id}`, values);
	});

	function reset() {
		forget(`projection.${id}`);
		({ monthly, rate, years, real } = defaults());
	}

	let history = $state<{ date: string; value: number }[]>([]);
	$effect(() => {
		if (!loadHistory) return;
		let stale = false;
		loadHistory()
			.then((pts) => {
				if (!stale) history = pts.map((p) => ({ date: p.date, value: p.value }));
			})
			.catch(() => {
				// The projection stands on its own without the history.
			});
		return () => {
			stale = true;
		};
	});

	let now = today();
	let months = $derived(Math.max(1, Math.round(years * 12)));
	let inflation = $derived(real && !isSavings ? INFLATION : 0);
	let base = $derived(projectGrowth({ start, monthly, annualReturn: rate / 100, months, inflation }));
	let low = $derived(
		isSavings ? null : projectGrowth({ start, monthly, annualReturn: Math.max(0, rate - SPREAD) / 100, months, inflation })
	);
	let high = $derived(
		isSavings ? null : projectGrowth({ start, monthly, annualReturn: (rate + SPREAD) / 100, months, inflation })
	);

	let rows = $derived(
		base.map((p, i) => ({
			date: addMonths(now, p.month),
			contributed: p.contributed,
			growth: p.growth,
			low: low?.[i].balance ?? 0,
			high: high?.[i].balance ?? 0
		}))
	);

	let end = $derived(base[base.length - 1]);
	let firstYearGrowth = $derived(base[Math.min(12, base.length - 1)].growth * (12 / Math.min(12, base.length - 1)));

	// The next round number or two ahead, and when it's crossed.
	let milestones = $derived.by(() => {
		const out: { amount: number; date: string; months: number }[] = [];
		for (const amount of MILESTONES) {
			if (amount <= start) continue;
			const hit = base.find((p) => p.balance >= amount);
			if (!hit) break;
			out.push({ amount, date: addMonths(now, hit.month), months: hit.month });
			if (out.length === 3) break;
		}
		return out;
	});

	let band = $derived(
		isSavings
			? null
			: {
					key: 'range',
					label: `At ${Math.max(0, rate - SPREAD)}–${rate + SPREAD}% a year`,
					color: SERIES.green,
					lo: 'low',
					hi: 'high'
				}
	);
	let stack = $derived([
		{ key: 'contributed', label: 'Put in', color: SERIES.blue },
		{ key: 'growth', label: isSavings ? 'Interest' : 'Growth', color: SERIES.green }
	]);
</script>

<div class="card">
	<div class="head">
		<div>
			<h2>{kind === 'retirement' ? 'Retirement projection' : 'Projection'}</h2>
			<p class="muted sub">
				{#if isSavings}
					At {rate}% APY, compounding monthly, adding {formatCurrencyWhole(monthly)} a month.
				{:else}
					A steady {rate}% a year, adding {formatCurrencyWhole(monthly)} a month. Real markets swing;
					the shaded range is {SPREAD} points either side.
				{/if}
			</p>
		</div>
		<Button variant="ghost" size="sm" onclick={reset}>Reset</Button>
	</div>

	<div class="stats">
		<div>
			<span class="label">In {formatDuration(months)}</span>
			<span class="value">{formatCurrencyWhole(end.balance)}</span>
			{#if low && high}
				<span class="muted small">
					{formatCurrencyWhole(low[low.length - 1].balance)} – {formatCurrencyWhole(high[high.length - 1].balance)}
				</span>
			{:else}
				<span class="muted small">by {formatDate(addMonths(now, months))}</span>
			{/if}
		</div>
		<div>
			<span class="label">You put in</span>
			<span class="value">{formatCurrencyWhole(end.contributed)}</span>
			<span class="muted small">{formatCurrencyWhole(start)} today + contributions</span>
		</div>
		<div>
			<span class="label">{isSavings ? 'Interest' : 'Growth'}</span>
			<span class="value pos">{formatCurrencyWhole(end.growth)}</span>
			<span class="muted small">≈ {formatCurrency(firstYearGrowth / 12)} a month to start</span>
		</div>
	</div>

	<ProjectionChart
		{history}
		historyLabel={isSavings ? 'Balance so far' : 'Value so far'}
		{rows}
		{stack}
		{band}
	/>

	{#if milestones.length}
		<ul class="milestones">
			{#each milestones as m (m.amount)}
				<li>
					<strong>{formatCurrencyWhole(m.amount)}</strong>
					<span class="muted">{formatDate(m.date)} · {formatDuration(m.months)}</span>
				</li>
			{/each}
		</ul>
	{/if}

	<div class="controls">
		<SliderField
			label="Added each month"
			id="{id}-monthly"
			bind:value={monthly}
			min={0}
			max={Math.max(2000, Math.ceil((defaultMonthly * 3) / 100) * 100)}
			step={25}
			prefix="$"
			hint={defaultMonthly > 0 ? `Your recurring transfers add ${formatCurrencyWhole(defaultMonthly)}` : undefined}
		/>
		<SliderField
			label={isSavings ? 'APY' : 'Yearly return'}
			id="{id}-rate"
			bind:value={rate}
			min={0}
			max={isSavings ? 8 : 12}
			step={isSavings ? 0.05 : 0.25}
			suffix="%"
			hint={isSavings
				? defaultRate != null
					? `Current rate ${defaultRate}%`
					: undefined
				: 'US stocks have averaged roughly 10% a year, about 7% after inflation'}
		/>
		<SliderField
			label={kind === 'retirement' ? 'Years until retirement' : 'Years'}
			id="{id}-years"
			bind:value={years}
			min={1}
			max={isSavings ? 15 : 45}
			step={1}
			suffix="yr"
		/>
		{#if !isSavings}
			<label class="toggle">
				<input type="checkbox" bind:checked={real} />
				<span>
					Show in today's dollars
					<span class="muted small">(less {INFLATION * 100}% inflation a year)</span>
				</span>
			</label>
		{/if}
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

	.small {
		font-size: 0.75rem;
	}

	.milestones {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		list-style: none;
		margin: 1rem 0 0;
		padding: 0;
	}

	.milestones li {
		display: flex;
		flex-direction: column;
		padding: 0.375rem 0.75rem;
		border: 1px solid var(--divider);
		border-radius: var(--radius-sm);
		font-size: 0.8125rem;
		font-variant-numeric: tabular-nums;
	}

	.milestones .muted {
		font-size: 0.75rem;
	}

	.controls {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem 1.5rem;
		margin-top: 1.25rem;
		padding-top: 1rem;
		border-top: 1px solid var(--divider);
	}

	.toggle {
		grid-column: 1 / -1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		cursor: pointer;
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
