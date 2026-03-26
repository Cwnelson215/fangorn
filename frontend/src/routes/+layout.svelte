<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	let { children }: { children: Snippet } = $props();
	let authChecked = $state(false);
	let isLoginPage = $derived(page.url.pathname === '/login');

	onMount(async () => {
		if (isLoginPage) {
			authChecked = true;
			return;
		}

		try {
			const res = await fetch('/api/auth/status');
			const data = await res.json();
			if (data.required && !data.authenticated) {
				goto('/login');
				return;
			}
		} catch {
			// If auth check fails, allow access (server may not require auth)
		}
		authChecked = true;
	});
</script>

{#if isLoginPage}
	{@render children()}
{:else if authChecked}
	<div class="app">
		<nav>
			<div class="nav-brand">Fangorn</div>
			<div class="nav-links">
				<a href="/">Dashboard</a>
				<a href="/accounts">Accounts</a>
				<a href="/transactions">Transactions</a>
				<a href="/transfers">Transfers</a>
				<a href="/import">Import</a>
			</div>
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

	:global(body) {
		font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
		background: #f8f9fa;
		color: #1a1a2e;
		line-height: 1.6;
	}

	.app {
		min-height: 100vh;
	}

	nav {
		background: #1a1a2e;
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
		color: #4ecca3;
	}

	.nav-links {
		display: flex;
		gap: 1.5rem;
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

	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}
</style>
