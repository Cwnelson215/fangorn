<script lang="ts">
	import type { AccountGroupBy } from '$lib/grouping';
	import { grouping, setGrouping } from '$lib/grouping.svelte';

	const OPTIONS: { value: AccountGroupBy; label: string }[] = [
		{ value: 'type', label: 'Type' },
		{ value: 'institution', label: 'Institution' }
	];
</script>

<div class="toggle" role="radiogroup" aria-label="Group accounts by">
	{#each OPTIONS as o (o.value)}
		<button
			type="button"
			role="radio"
			aria-checked={grouping.by === o.value}
			class:active={grouping.by === o.value}
			onclick={() => setGrouping(o.value)}
		>
			{o.label}
		</button>
	{/each}
</div>

<style>
	.toggle {
		display: inline-grid;
		grid-template-columns: repeat(2, auto);
		background: var(--bg);
		border-radius: var(--radius-sm);
		padding: 0.1875rem;
		gap: 0.1875rem;
	}

	.toggle button {
		min-height: 2rem;
		padding: 0.25rem 0.75rem;
		border: none;
		background: none;
		border-radius: 6px;
		font: inherit;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--muted);
		cursor: pointer;
	}

	.toggle button.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow);
	}
</style>
