<script lang="ts">
	// A budget's progress bar, shared by the dashboard and the budgets page. While
	// the month is in progress it also marks how much of the month has passed, so
	// "$300 of $400" on the 10th reads as a warning rather than "$100 left".
	// Recurring charges still to post show as a striped segment after the spend.
	import type { BudgetPace } from '$lib/budget';

	let {
		spent,
		amount,
		scheduled = 0,
		color,
		pace
	}: {
		spent: number;
		amount: number;
		scheduled?: number;
		color?: string | null;
		pace: BudgetPace;
	} = $props();

	const pct = (v: number) => (amount <= 0 ? 0 : Math.min(100, Math.max(0, (v / amount) * 100)));

	let fill = $derived(
		pace.status === 'over'
			? 'var(--neg)'
			: pace.status === 'committed' || pace.status === 'ahead'
				? 'var(--warn)'
				: (color ?? 'var(--accent)')
	);
	let spentPct = $derived(pct(spent));
	// Clipped to what's left of the bar, so an over-committed budget fills it exactly.
	let scheduledPct = $derived(pct(spent + scheduled) - spentPct);
</script>

<div class="track">
	<div class="bar">
		<div class="fill" style="width: {spentPct}%; background: {fill}"></div>
		{#if scheduledPct > 0}
			<div
				class="scheduled"
				style="width: {scheduledPct}%; --fill: {fill}"
				title="Scheduled recurring charges"
			></div>
		{/if}
	</div>
	{#if pace.elapsed !== null}
		<div
			class="today"
			style="left: {pace.elapsed * 100}%"
			title="Today: {Math.round(pace.elapsed * 100)}% through the month"
		></div>
	{/if}
</div>

<style>
	.track {
		position: relative;
	}

	.bar {
		display: flex;
		height: 8px;
		background: var(--divider);
		border-radius: 999px;
		overflow: hidden;
	}

	.fill,
	.scheduled {
		height: 100%;
		transition: width 0.3s;
	}

	.scheduled {
		background: repeating-linear-gradient(
			-45deg,
			var(--fill) 0 3px,
			transparent 3px 6px
		);
		opacity: 0.7;
	}

	.today {
		position: absolute;
		top: -3px;
		bottom: -3px;
		width: 2px;
		margin-left: -1px;
		background: var(--ink);
		opacity: 0.45;
		border-radius: 1px;
	}
</style>
