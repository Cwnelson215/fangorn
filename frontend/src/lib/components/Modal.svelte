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
		background: rgba(26, 26, 46, 0.45);
		display: flex;
		align-items: flex-start;
		justify-content: center;
		padding: 2rem 1rem;
		z-index: 200;
		overflow-y: auto;
	}

	.dialog {
		background: var(--surface);
		border-radius: var(--radius);
		box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
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
</style>
