<script lang="ts">
	// Wraps a labelled form control. Every form in the app used to hand-roll this
	// div/label/input trio with its own copy of the styles.
	import type { Snippet } from 'svelte';

	let {
		label,
		id,
		hint,
		children
	}: {
		label: string;
		id: string;
		hint?: string;
		children: Snippet;
	} = $props();
</script>

<div class="field">
	<label for={id}>{label}</label>
	{@render children()}
	{#if hint}
		<span class="hint">{hint}</span>
	{/if}
</div>

<style>
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		flex: 1;
		min-width: 0;
	}

	label {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--muted);
	}

	.hint {
		font-size: 0.75rem;
		color: var(--muted-light);
	}

	/* Styling the slotted control from here keeps every input in the app
	   consistent without each page repeating the rules. */
	.field :global(input),
	.field :global(select),
	.field :global(textarea) {
		width: 100%;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		font: inherit;
		font-size: 0.9375rem;
		background: var(--surface);
		color: var(--ink);
	}

	.field :global(input:focus),
	.field :global(select:focus),
	.field :global(textarea:focus) {
		outline: none;
		border-color: var(--accent);
	}

	.field :global(input:disabled),
	.field :global(select:disabled),
	.field :global(textarea:disabled) {
		background: var(--bg);
		color: var(--muted);
		cursor: not-allowed;
	}

	.field :global(textarea) {
		resize: vertical;
		min-height: 4rem;
	}
</style>
