<script lang="ts">
	import type { ValuePoint } from '$lib/types';
	import { formatSigned, formatPercent } from '$lib/format';
	import ValueChart from './ValueChart.svelte';

	// "Value over time" for one investment account or all of them. The series is
	// rebuilt from the trade log server-side, so it isn't polled: only its last
	// point moves between trades.
	let { load, id }: { load: (days: number) => Promise<ValuePoint[]>; id: string } = $props();

	const RANGES = [
		{ label: '1M', days: 30 },
		{ label: '3M', days: 91 },
		{ label: '1Y', days: 365 },
		{ label: 'All', days: 0 }
	];

	let days = $state(91);
	let points = $state<ValuePoint[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Re-runs when the range changes. A slower earlier request can't overwrite a
	// newer one: each run ignores its result once it has been superseded.
	$effect(() => {
		const range = days;
		let stale = false;
		loading = true;
		load(range)
			.then((p) => {
				if (stale) return;
				points = p;
				error = null;
			})
			.catch((e) => {
				if (!stale) error = e instanceof Error ? e.message : 'Could not load the history';
			})
			.finally(() => {
				if (!stale) loading = false;
			});
		return () => {
			stale = true;
		};
	});

	let change = $derived(points.length >= 2 ? points[points.length - 1].value - points[0].value : null);
	let changePct = $derived(
		change != null && points[0].value > 0 ? change / points[0].value : null
	);
</script>

<div class="card">
	<div class="head">
		<div>
			<h2>Value over time</h2>
			{#if change != null}
				<span class="muted" class:pos={change > 0.004} class:neg={change < -0.004}>
					{formatSigned(change)}
					{#if changePct != null}({formatPercent(changePct, true)}){/if}
				</span>
				<span class="muted">over this range, including money moved in or out</span>
			{/if}
		</div>
		<div class="ranges" role="group" aria-label="Range">
			{#each RANGES as r (r.label)}
				<button class:active={days === r.days} onclick={() => (days = r.days)}>{r.label}</button>
			{/each}
		</div>
	</div>

	{#if error}
		<p class="error-text">{error}</p>
	{:else if points.length >= 2}
		<div class:dim={loading}>
			<ValueChart {points} {id} />
		</div>
	{:else if !loading}
		<p class="muted">Not enough history yet — check back tomorrow.</p>
	{:else}
		<p class="muted">Loading…</p>
	{/if}
</div>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}

	h2 {
		font-size: 1rem;
	}

	.head .muted {
		font-size: 0.8125rem;
	}

	.ranges {
		display: inline-flex;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}

	.ranges button {
		font: inherit;
		font-size: 0.8125rem;
		font-weight: 600;
		padding: 0.25rem 0.75rem;
		border: none;
		background: var(--surface);
		color: var(--muted);
		cursor: pointer;
	}

	.ranges button + button {
		border-left: 1px solid var(--border);
	}

	.ranges button.active {
		background: var(--ink);
		color: var(--surface);
	}

	.dim {
		opacity: 0.5;
		transition: opacity 0.15s;
	}
</style>
