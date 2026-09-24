<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authStatus, logout } from '$lib/api';

	let { children }: { children: Snippet } = $props();
	let authChecked = $state(false);
	let showLogout = $state(false);
	let isLoginPage = $derived(page.url.pathname === '/login');

	type Icon = 'home' | 'list' | 'plus' | 'target' | 'menu' | 'wallet' | 'chart' | 'receipt' | 'swap' | 'repeat' | 'tag';

	const NAV: { href: string; label: string; icon: Icon }[] = [
		{ href: '/', label: 'Dashboard', icon: 'home' },
		{ href: '/accounts', label: 'Accounts', icon: 'wallet' },
		{ href: '/investments', label: 'Investments', icon: 'chart' },
		{ href: '/transactions', label: 'Transactions', icon: 'list' },
		{ href: '/receipts', label: 'Receipts', icon: 'receipt' },
		{ href: '/transfers', label: 'Transfers', icon: 'swap' },
		{ href: '/recurring', label: 'Recurring', icon: 'repeat' },
		{ href: '/budgets', label: 'Budgets', icon: 'target' },
		{ href: '/categories', label: 'Categories', icon: 'tag' }
	];

	// On a phone the bottom bar has room for four destinations and the add
	// button; everything else lives behind More.
	const TABS = ['/', '/transactions', '/budgets'];
	const MORE = NAV.filter((item) => !TABS.includes(item.href));

	// Quick add is the reason to open the app on a phone, so it is one tap from
	// every page. The target pages open their entry form on ?new.
	const QUICK_ADD = [
		{ href: '/transactions?new', label: 'Log a transaction', hint: 'Money in, out, or a refund', icon: 'list' },
		{ href: '/receipts', label: 'Scan a receipt', hint: 'Take a photo, it posts itself', icon: 'receipt' },
		{ href: '/transfers?new', label: 'Record a transfer', hint: 'Between your own accounts', icon: 'swap' }
	] as const;

	let sheet = $state<'more' | 'add' | null>(null);
	let moreActive = $derived(MORE.some((item) => isActive(item.href)));

	// Any navigation, including the browser back button, dismisses the sheet.
	// href rather than pathname: quick add can target the page already open.
	$effect(() => {
		page.url.href;
		sheet = null;
	});

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') sheet = null;
	}

	function isActive(href: string): boolean {
		if (href === '/') return page.url.pathname === '/';
		return page.url.pathname.startsWith(href);
	}

	// The layout stays mounted across client-side navigation, so onMount only ever
	// saw the first page. Checking per route means arriving from /login re-verifies
	// instead of rendering nothing.
	$effect(() => {
		if (isLoginPage) {
			// Leaving /login (after signing in or out) must re-check.
			authChecked = false;
			return;
		}
		if (authChecked) return;

		const path = page.url.pathname;
		authStatus()
			.then((data) => {
				if (page.url.pathname !== path) return; // stale: navigated away meanwhile
				showLogout = data.required;
				if (data.required && !data.authenticated) {
					goto('/login');
					return;
				}
				authChecked = true;
			})
			.catch(() => {
				// Fail closed. A failed auth check used to fall through and render the
				// app, which meant a network blip looked identical to being signed in.
				goto('/login');
			});
	});

	async function handleLogout() {
		sheet = null;
		try {
			await logout();
		} finally {
			goto('/login');
		}
	}
</script>

{#snippet glyph(name: Icon)}
	<svg viewBox="0 0 24 24" aria-hidden="true">
		{#if name === 'home'}
			<path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6h-6v6H4a1 1 0 0 1-1-1z" />
		{:else if name === 'list'}
			<path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01" />
		{:else if name === 'plus'}
			<path d="M12 5v14M5 12h14" />
		{:else if name === 'target'}
			<circle cx="12" cy="12" r="9" /><circle cx="12" cy="12" r="5" /><circle cx="12" cy="12" r="1" />
		{:else if name === 'menu'}
			<path d="M4 6h16M4 12h16M4 18h16" />
		{:else if name === 'wallet'}
			<path d="M3 7a2 2 0 0 1 2-2h14v4M3 7v11a2 2 0 0 0 2 2h16V9H5a2 2 0 0 1-2-2zM17 14.5h.01" />
		{:else if name === 'chart'}
			<path d="M3 3v18h18M7 15l4-4 3 3 6-6" />
		{:else if name === 'receipt'}
			<path d="M5 3h14v18l-3-2-2 2-2-2-2 2-2-2-3 2zM9 8h6M9 12h6" />
		{:else if name === 'swap'}
			<path d="M7 4 3 8l4 4M3 8h14M17 12l4 4-4 4M21 16H7" />
		{:else if name === 'repeat'}
			<path d="M17 2l4 4-4 4M3 11V9a3 3 0 0 1 3-3h15M7 22l-4-4 4-4M21 13v2a3 3 0 0 1-3 3H3" />
		{:else if name === 'tag'}
			<path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8zM7.5 7.5h.01" />
		{/if}
	</svg>
{/snippet}

<svelte:window onkeydown={sheet ? onKeydown : undefined} />

{#if isLoginPage}
	{@render children()}
{:else if authChecked}
	<div class="app">
		<nav class="topbar">
			<a class="nav-brand" href="/">Fangorn</a>
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

		<nav class="tabbar" aria-label="Main">
			<a href="/" class:active={isActive('/')}>{@render glyph('home')}<span>Home</span></a>
			<a href="/transactions" class:active={isActive('/transactions')}>
				{@render glyph('list')}<span>Activity</span>
			</a>
			<button
				class="add"
				class:open={sheet === 'add'}
				aria-label="Add"
				aria-expanded={sheet === 'add'}
				onclick={() => (sheet = sheet === 'add' ? null : 'add')}
			>
				{@render glyph('plus')}
			</button>
			<a href="/budgets" class:active={isActive('/budgets')}>{@render glyph('target')}<span>Budgets</span></a>
			<button
				class:active={moreActive || sheet === 'more'}
				aria-expanded={sheet === 'more'}
				onclick={() => (sheet = sheet === 'more' ? null : 'more')}
			>
				{@render glyph('menu')}<span>More</span>
			</button>
		</nav>

		{#if sheet}
			<!-- Same click-to-dismiss backdrop pattern as Modal; Escape is handled on the window. -->
			<div class="sheet-backdrop" role="presentation" onclick={() => (sheet = null)}></div>
			<div class="sheet" role="dialog" aria-modal="true" aria-label={sheet === 'add' ? 'Add' : 'More'}>
				{#if sheet === 'add'}
					{#each QUICK_ADD as item (item.href)}
						<a class="sheet-item" href={item.href}>
							<span class="sheet-icon accent">{@render glyph(item.icon)}</span>
							<span class="sheet-text">
								<span>{item.label}</span>
								<span class="muted">{item.hint}</span>
							</span>
						</a>
					{/each}
				{:else}
					{#each MORE as item (item.href)}
						<a class="sheet-item" href={item.href} class:active={isActive(item.href)}>
							<span class="sheet-icon">{@render glyph(item.icon)}</span>
							<span class="sheet-text"><span>{item.label}</span></span>
						</a>
					{/each}
					{#if showLogout}
						<button class="sheet-item signout" onclick={handleLogout}>
							<span class="sheet-text"><span>Sign out</span></span>
						</button>
					{/if}
				{/if}
			</div>
		{/if}
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

	/* --warn itself is too light to read as text on white. */
	:global(.warn-text) {
		color: #b45309;
	}

	/* Phone-first global adjustments. 16px is the size below which iOS Safari
	   zooms the page when an input takes focus. */
	:global(html) {
		-webkit-text-size-adjust: 100%;
	}

	:global(body) {
		-webkit-tap-highlight-color: transparent;
	}

	:global(button),
	:global(a) {
		touch-action: manipulation;
	}

	/* Two fields share a row until the row gets too narrow for both, then they
	   stack. Every form in the app uses .form-row for its paired fields. */
	:global(.form-row) {
		flex-wrap: wrap;
	}

	:global(.form-row > .field) {
		flex: 1 1 9rem;
	}

	/* Day headings split a list of rows on a phone, where the date column is
	   dropped. On wider screens the rows show their own date. */
	:global(.day-heading) {
		display: none;
	}

	/* Column headers of the desktop tables mean nothing once rows stack. */
	@media (max-width: 639px) {
		:global(.day-heading) {
			display: block;
			position: sticky;
			top: calc(48px + env(safe-area-inset-top));
			z-index: 1;
			margin: 0 -1rem;
			padding: 0.375rem 1.25rem;
			background: var(--bg);
			font-size: 0.75rem;
			font-weight: 600;
			color: var(--muted);
			text-transform: uppercase;
			letter-spacing: 0.03em;
		}

		:global(.day-heading:first-child) {
			margin-top: -1rem;
			border-radius: var(--radius) var(--radius) 0 0;
		}

		:global(.table-head) {
			display: none !important;
		}

		:global(.table-scroll) {
			overflow-x: visible !important;
		}

		:global(.table-scroll > *) {
			min-width: 0 !important;
		}
	}

	.app {
		min-height: 100vh;
		min-height: 100dvh;
	}

	.topbar {
		background: var(--ink);
		color: white;
		padding: 0 2rem;
		padding-top: env(safe-area-inset-top);
		height: calc(60px + env(safe-area-inset-top));
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
		text-decoration: none;
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

	.tabbar,
	.sheet,
	.sheet-backdrop {
		display: none;
	}

	svg {
		width: 24px;
		height: 24px;
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	/* ---- Phone ---------------------------------------------------------- */

	@media (max-width: 899px) {
		:global(.card) {
			padding: 1rem;
		}

		:global(.page) {
			gap: 1rem;
		}

		:global(.page-header h1) {
			font-size: 1.375rem;
		}

		.topbar {
			padding-left: max(1rem, env(safe-area-inset-left));
			padding-right: max(1rem, env(safe-area-inset-right));
			height: calc(48px + env(safe-area-inset-top));
		}

		.nav-links,
		.logout {
			display: none;
		}

		.nav-brand {
			font-size: 1.125rem;
		}

		main {
			padding: 1rem;
			padding-left: max(1rem, env(safe-area-inset-left));
			padding-right: max(1rem, env(safe-area-inset-right));
			/* Clear the fixed tab bar. */
			padding-bottom: calc(64px + 1.5rem + env(safe-area-inset-bottom));
		}

		.tabbar {
			display: grid;
			grid-template-columns: repeat(5, 1fr);
			align-items: center;
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			z-index: 150;
			height: calc(64px + env(safe-area-inset-bottom));
			padding: 0 env(safe-area-inset-right) env(safe-area-inset-bottom) env(safe-area-inset-left);
			background: var(--surface);
			border-top: 1px solid var(--divider);
		}

		.tabbar a,
		.tabbar button {
			display: flex;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			gap: 2px;
			height: 100%;
			background: none;
			border: none;
			font: inherit;
			font-size: 0.6875rem;
			font-weight: 600;
			color: var(--muted-light);
			text-decoration: none;
			cursor: pointer;
		}

		.tabbar .active {
			color: var(--ink);
		}

		.tabbar .active svg {
			stroke: var(--accent-hover);
		}

		.tabbar .add {
			width: 52px;
			height: 52px;
			margin: 0 auto;
			border-radius: 50%;
			background: var(--accent);
			color: var(--ink);
			box-shadow: 0 4px 12px rgba(78, 204, 163, 0.45);
			transition: transform 0.15s;
		}

		.tabbar .add svg {
			width: 28px;
			height: 28px;
			stroke-width: 2.5;
		}

		.tabbar .add.open {
			transform: rotate(45deg);
		}

		.sheet-backdrop {
			display: block;
			position: fixed;
			inset: 0;
			z-index: 140;
			background: rgba(26, 26, 46, 0.45);
		}

		.sheet {
			display: flex;
			flex-direction: column;
			position: fixed;
			left: 0;
			right: 0;
			bottom: calc(64px + env(safe-area-inset-bottom));
			z-index: 145;
			max-height: calc(100dvh - 64px - 4rem);
			overflow-y: auto;
			padding: 0.5rem max(0.5rem, env(safe-area-inset-right)) 0.5rem max(0.5rem, env(safe-area-inset-left));
			background: var(--surface);
			border-radius: var(--radius) var(--radius) 0 0;
			box-shadow: 0 -8px 24px rgba(0, 0, 0, 0.12);
			animation: rise 0.18s ease-out;
		}

		.sheet-item {
			display: flex;
			align-items: center;
			gap: 0.875rem;
			min-height: 52px;
			padding: 0.5rem 0.75rem;
			border: none;
			border-radius: var(--radius-sm);
			background: none;
			font: inherit;
			font-size: 1rem;
			font-weight: 500;
			color: var(--ink);
			text-align: left;
			text-decoration: none;
			cursor: pointer;
		}

		.sheet-item:active {
			background: var(--bg);
		}

		.sheet-item.active {
			color: var(--accent-hover);
		}

		.sheet-icon {
			display: grid;
			place-items: center;
			width: 36px;
			height: 36px;
			border-radius: 10px;
			background: var(--bg);
			color: var(--muted);
			flex-shrink: 0;
		}

		.sheet-icon svg {
			width: 20px;
			height: 20px;
		}

		.sheet-icon.accent {
			background: #e6f8f1;
			color: #1f8a68;
		}

		.sheet-item.active .sheet-icon {
			color: var(--accent-hover);
		}

		.sheet-text {
			display: flex;
			flex-direction: column;
			line-height: 1.3;
		}

		.sheet-text .muted {
			font-size: 0.8125rem;
		}

		.signout {
			margin-top: 0.25rem;
			border-top: 1px solid var(--divider);
			border-radius: 0;
			color: var(--neg);
		}
	}

	@keyframes rise {
		from {
			transform: translateY(12px);
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sheet {
			animation: none;
		}
	}
</style>
