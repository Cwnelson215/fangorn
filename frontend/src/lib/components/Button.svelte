<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		variant = 'primary',
		type = 'button',
		disabled = false,
		size = 'md',
		onclick,
		children
	}: {
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
		type?: 'button' | 'submit';
		disabled?: boolean;
		size?: 'sm' | 'md';
		onclick?: (event: MouseEvent) => void;
		children: Snippet;
	} = $props();
</script>

<button {type} {disabled} {onclick} class="btn {variant} {size}">
	{@render children()}
</button>

<style>
	.btn {
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		font: inherit;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
		transition:
			background 0.15s,
			border-color 0.15s,
			color 0.15s;
	}

	.md {
		padding: 0.5rem 1rem;
		font-size: 0.9375rem;
	}

	.sm {
		padding: 0.3125rem 0.625rem;
		font-size: 0.8125rem;
	}

	.primary {
		background: var(--accent);
		color: var(--on-accent);
	}

	.primary:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.secondary {
		background: var(--surface);
		border-color: var(--border);
		color: var(--ink);
	}

	.secondary:hover:not(:disabled) {
		border-color: var(--muted);
	}

	.ghost {
		background: none;
		color: var(--muted);
	}

	.ghost:hover:not(:disabled) {
		color: var(--ink);
	}

	.danger {
		background: none;
		border-color: transparent;
		color: var(--neg);
	}

	.danger:hover:not(:disabled) {
		background: var(--neg-soft);
	}

	@media (pointer: coarse) {
		.md {
			min-height: 44px;
		}

		.sm {
			min-height: 36px;
			padding: 0.375rem 0.75rem;
		}
	}

	.btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
</style>
