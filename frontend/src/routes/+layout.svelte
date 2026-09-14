<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { authStatus, logout } from '$lib/api';

	let { children }: { children: Snippet } = $props();
	let authChecked = $state(false);
	let showLogout = $state(false);
	let isLoginPage = $derived(page.url.pathname === '/login');

	const NAV = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/accounts', label: 'Accounts' },
		{ href: '/transactions', label: 'Transactions' },
		{ href: '/transfers', label: 'Transfers' },
		{ href: '/recurring', label: 'Recurring' },
		{ href: '/budgets', label: 'Budgets' },
		{ href: '/categories', label: 'Categories' }
	];

	function isActive(href: string): boolean {
		if (href === '/') return page.url.pathname === '/';
		return page.url.pathname.startsWith(href);
	}

	onMount(async () => {
		if (isLoginPage) {
			authChecked = true;
			return;
		}

		try {
			const data = await authStatus();
			showLogout = data.required;
			if (data.required && !data.authenticated) {
				goto('/login');
				return;
			}
		} catch {
			// Fail closed. A failed auth check used to fall through and render the
			// app, which meant a network blip looked identical to being signed in.
			goto('/login');
			return;
		}
		authChecked = true;
	});

	async function handleLogout() {
		try {
			await logout();
		} finally {
			goto('/login');
		}
	}
</script>

{#if isLoginPage}
	{@render children()}
{:else if authChecked}
	<div class="app">
		<nav>
			<div class="nav-brand">Fangorn</div>
			<div class="nav-links">
				{#each NAV as item}
					<a href={item.href} class:active={isActive(item.href)}>{item.label}</a>
				{/each}
			</div>
			{#if showLogout}
				<button class="logout" onclick={handleLogout}>Sign out</button>
			{/if}
		</nav>
		<main>
			{@render children()}
		</main>
	</div>
{/if}

<style>
	:global(*) {
		margin: 0;
		padding: 0;
		box-sizing: border-box;
	}

	/* Design tokens. Every colour in the app resolves through these — before the
	   pivot the same hex values were copy-pasted across a dozen components. */
	:global(:root) {
		--accent: #4ecca3;
		--accent-hover: #3db88f;
		--ink: #1a1a2e;
		--bg: #f8f9fa;
		--surface: #ffffff;
		--muted: #666;
		--muted-light: #999;
		--border: #ddd;
		--divider: #e5e7eb;
		--pos: #22c55e;
		--neg: #ef4444;
		--info: #3b82f6;
		--warn: #f59e0b;

		--radius: 12px;
		--radius-sm: 8px;
		--shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	:global(body) {
		font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
		background: var(--bg);
		color: var(--ink);
		line-height: 1.6;
	}

	/* Shared primitives, defined once so pages stop redeclaring them. */
	:global(.card) {
		background: var(--surface);
		border-radius: var(--radius);
		padding: 1.5rem;
		box-shadow: var(--shadow);
	}

	:global(.page) {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	:global(.page-header) {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		flex-wrap: wrap;
	}

	:global(.page-header h1) {
		font-size: 1.5rem;
	}

	:global(.muted) {
		color: var(--muted);
		font-size: 0.875rem;
	}

	:global(.empty) {
		text-align: center;
		padding: 3rem 1.5rem;
		color: var(--muted);
	}

	:global(.error-text) {
		color: var(--neg);
		font-size: 0.875rem;
	}

	:global(.pos) {
		color: var(--pos);
	}

	:global(.neg) {
		color: var(--neg);
	}

	.app {
		min-height: 100vh;
	}

	nav {
		background: var(--ink);
		color: white;
		padding: 0 2rem;
		height: 60px;
		display: flex;
		align-items: center;
		gap: 2rem;
		position: sticky;
		top: 0;
		z-index: 100;
	}

	.nav-brand {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--accent);
	}

	.nav-links {
		display: flex;
		gap: 1.5rem;
		flex-wrap: wrap;
	}

	.nav-links a {
		color: rgba(255, 255, 255, 0.7);
		text-decoration: none;
		font-size: 0.9rem;
		font-weight: 500;
		transition: color 0.2s;
	}

	.nav-links a:hover {
		color: white;
	}

	.nav-links a.active {
		color: var(--accent);
	}

	.logout {
		margin-left: auto;
		background: none;
		border: none;
		color: rgba(255, 255, 255, 0.6);
		font: inherit;
		font-size: 0.85rem;
		cursor: pointer;
		padding: 0.25rem 0;
	}

	.logout:hover {
		color: white;
	}

	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}
</style>
