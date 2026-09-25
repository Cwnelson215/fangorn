<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		open = $bindable(false),
		onclose,
		children
	}: {
		title: string;
		open?: boolean;
		onclose?: () => void;
		children: Snippet;
	} = $props();

	function close() {
		open = false;
		onclose?.();
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') close();
	}

	// Without this, a swipe that reaches the end of a long form on a phone keeps
	// going and scrolls the page underneath.
	$effect(() => {
		if (!open) return;
		const previous = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		return () => {
			document.body.style.overflow = previous;
		};
	});
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
	<!-- The backdrop is click-to-dismiss. It carries a keyboard handler and
	     role/tabindex purely to satisfy a11y linting; Escape is handled globally
	     above so focus never has to be on the backdrop itself. -->
	<div
		class="backdrop"
		role="button"
		tabindex="-1"
		aria-label="Close dialog"
		onclick={close}
		onkeydown={onKeydown}
	>
		<!-- Stop clicks inside the dialog from reaching the backdrop. -->
		<div
			class="dialog"
			role="dialog"
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<header>
				<h2>{title}</h2>
				<button class="close" onclick={close} aria-label="Close">×</button>
			</header>
			<div class="body">
				{@render children()}
			</div>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: var(--overlay);
		display: flex;
		align-items: flex-start;
		justify-content: center;
		padding: 2rem 1rem;
		z-index: 200;
		overflow-y: auto;
		overscroll-behavior: contain;
	}

	.dialog {
		background: var(--surface);
		border-radius: var(--radius);
		border: 1px solid var(--border);
		box-shadow: var(--shadow-lg);
		width: 100%;
		max-width: 520px;
		margin: auto;
		cursor: default;
	}

	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--divider);
	}

	h2 {
		font-size: 1.125rem;
	}

	.close {
		background: none;
		border: none;
		font-size: 1.5rem;
		line-height: 1;
		color: var(--muted);
		cursor: pointer;
		padding: 0 0.25rem;
	}

	.close:hover {
		color: var(--ink);
	}

	.body {
		padding: 1.5rem;
	}

	/* On a phone the dialog is a bottom sheet: full width, within thumb reach,
	   and scrolling inside itself so the header and close button stay put. */
	@media (max-width: 899px) {
		.backdrop {
			align-items: flex-end;
			padding: 0;
			overflow: hidden;
		}

		.dialog {
			max-width: none;
			margin: 0;
			border-radius: var(--radius) var(--radius) 0 0;
			max-height: calc(100dvh - env(safe-area-inset-top) - 1.5rem);
			display: flex;
			flex-direction: column;
			animation: rise 0.2s ease-out;
		}

		header {
			padding: 0.875rem 1rem;
			flex-shrink: 0;
		}

		.close {
			width: 44px;
			height: 44px;
			margin: -0.5rem -0.75rem -0.5rem 0;
		}

		.body {
			padding: 1rem max(1rem, env(safe-area-inset-right))
				calc(1rem + env(safe-area-inset-bottom)) max(1rem, env(safe-area-inset-left));
			overflow-y: auto;
			overscroll-behavior: contain;
		}
	}

	@keyframes rise {
		from {
			transform: translateY(24px);
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dialog {
			animation: none;
		}
	}
</style>
