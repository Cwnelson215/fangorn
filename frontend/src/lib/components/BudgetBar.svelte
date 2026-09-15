<script lang="ts">
	// A budget's progress bar, shared by the dashboard and the budgets page. While
	// the month is in progress it also marks how much of the month has passed, so
	// "$300 of $400" on the 10th reads as a warning rather than "$100 left".
	import type { BudgetPace } from '$lib/budget';

	let {
		spent,
		amount,
		color,
		pace
	}: {
		spent: number;
		amount: number;
		color?: string | null;
		pace: BudgetPace;
	} = $props();

	const pct = (v: number) => (amount <= 0 ? 0 : Math.min(100, Math.max(0, (v / amount) * 100)));

	let fill = $derived(
		pace.status === 'over'
			? 'var(--neg)'
			: pace.status === 'ahead'
				? 'var(--warn)'
				: (color ?? 'var(--accent)')
	);
</script>

<div class="track">
	<div class="bar">
		<div class="fill" style="width: {pct(spent)}%; background: {fill}"></div>
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
		height: 8px;
		background: var(--divider);
		border-radius: 999px;
		overflow: hidden;
	}

	.fill {
		height: 100%;
		border-radius: 999px;
		transition: width 0.3s;
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
