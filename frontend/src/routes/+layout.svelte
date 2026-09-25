<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, tick } from 'svelte';
	import { authStatus } from '$lib/api';
	import { capture, dismissNotice, openCamera, registerCameraInput, uploadPhoto } from '$lib/capture.svelte';
	import { isAppleMobile } from '$lib/device';

	let { children }: { children: Snippet } = $props();
	let authChecked = $state(false);
	let isLoginPage = $derived(page.url.pathname === '/login');

	type Icon = 'home' | 'list' | 'plus' | 'target' | 'menu' | 'wallet' | 'chart' | 'receipt' | 'swap' | 'repeat' | 'tag' | 'phone' | 'camera' | 'gear' | 'chevron' | 'trend';

	const NAV: { href: string; label: string; icon: Icon }[] = [
		{ href: '/', label: 'Dashboard', icon: 'home' },
		{ href: '/accounts', label: 'Accounts', icon: 'wallet' },
		{ href: '/investments', label: 'Investments', icon: 'chart' },
		{ href: '/whatif', label: 'What if', icon: 'trend' },
		{ href: '/transactions', label: 'Transactions', icon: 'list' },
		{ href: '/receipts', label: 'Receipts', icon: 'receipt' },
		{ href: '/transfers', label: 'Transfers', icon: 'swap' },
		{ href: '/recurring', label: 'Recurring', icon: 'repeat' },
		{ href: '/budgets', label: 'Budgets', icon: 'target' },
		{ href: '/categories', label: 'Categories', icon: 'tag' }
	];

	// On a wider screen the pages are grouped into three menus instead of nine
	// links in a row. Dashboard stays a plain link; Settings is the gear.
	type NavItem = (typeof NAV)[number];
	const pick = (href: string, label?: string): NavItem => {
		const item = NAV.find((n) => n.href === href)!;
		return label ? { ...item, label } : item;
	};
	const GROUPS: { id: string; label: string; items: NavItem[] }[] = [
		{ id: 'money', label: 'Money', items: [pick('/transactions'), pick('/receipts'), pick('/transfers')] },
		{
			id: 'plan',
			label: 'Plan',
			items: [pick('/budgets', 'Budgets & goals'), pick('/recurring'), pick('/categories')]
		},
		{ id: 'wealth', label: 'Wealth', items: [pick('/accounts'), pick('/investments'), pick('/whatif')] }
	];

	let openMenu = $state<string | null>(null);

	function groupActive(group: (typeof GROUPS)[number]): boolean {
		return group.items.some((item) => isActive(item.href));
	}

	function toggleMenu(id: string) {
		openMenu = openMenu === id ? null : id;
	}

	function menuItems(id: string): HTMLElement[] {
		return [...document.querySelectorAll<HTMLElement>(`#menu-${id} [role='menuitem']`)];
	}

	// Enter, Space or ArrowDown on a trigger opens its menu with the first item
	// focused, the way a native menu button behaves.
	async function onTriggerKeydown(event: KeyboardEvent, id: string) {
		if (event.key !== 'ArrowDown' && event.key !== 'Enter' && event.key !== ' ') return;
		event.preventDefault();
		openMenu = id;
		await tick();
		menuItems(id)[0]?.focus();
	}

	function onMenuKeydown(event: KeyboardEvent, id: string) {
		const items = menuItems(id);
		const at = items.indexOf(document.activeElement as HTMLElement);
		const move = (to: number) => {
			event.preventDefault();
			items[(to + items.length) % items.length]?.focus();
		};
		if (event.key === 'ArrowDown') move(at + 1);
		else if (event.key === 'ArrowUp') move(at - 1);
		else if (event.key === 'Home') move(0);
		else if (event.key === 'End') move(items.length - 1);
		else if (event.key === 'Escape') {
			event.preventDefault();
			openMenu = null;
			document.querySelector<HTMLElement>(`[aria-controls='menu-${id}']`)?.focus();
		}
	}

	// Tabbing or clicking away from a menu closes it.
	function onMenuFocusout(event: FocusEvent) {
		const wrap = event.currentTarget as HTMLElement;
		if (!wrap.contains(event.relatedTarget as Node | null)) openMenu = null;
	}

	function onWindowClick(event: MouseEvent) {
		if (openMenu && !(event.target as Element).closest('[data-menu]')) openMenu = null;
	}

	// Opens the camera from inside the menu item's own click, which is what lets
	// the browser show the file picker.
	function snapFromMenu() {
		openMenu = null;
		openCamera();
	}

	const SETTINGS = { href: '/settings', label: 'Settings', icon: 'gear' as Icon };
	const SHORTCUT = { href: '/shortcut', label: 'iPhone Shortcut', icon: 'phone' as Icon };

	// Shortcuts only run on iPhone and iPad, so nobody else is offered the setup.
	let onAppleMobile = $state(false);
	onMount(() => {
		onAppleMobile = isAppleMobile();
	});

	// On a phone the bottom bar has room for four destinations and the add
	// button; everything else lives behind More.
	const TABS = ['/', '/transactions', '/budgets'];
	let MORE = $derived([
		...NAV.filter((item) => !TABS.includes(item.href)),
		...(onAppleMobile ? [SHORTCUT] : []),
		SETTINGS
	]);

	let sheet = $state<'more' | null>(null);
	let moreActive = $derived(MORE.some((item) => isActive(item.href)));

	// Any navigation, including the browser back button, dismisses the sheet
	// and any open menu.
	$effect(() => {
		page.url.href;
		sheet = null;
		openMenu = null;
	});

	function onKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape') return;
		sheet = null;
		openMenu = null;
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

	function registerCamera(el: HTMLInputElement) {
		registerCameraInput(el);
		return { destroy: () => registerCameraInput(null) };
	}

	function onPhoto(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = ''; // so the same photo can be picked again
		if (file) uploadPhoto(file);
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
		{:else if name === 'camera'}
			<path d="M4 8h3l2-3h6l2 3h3v11H4z" /><circle cx="12" cy="13" r="3.5" />
		{:else if name === 'trend'}
			<path d="M3 17l6-6 4 4 8-8M15 7h6v6" />
		{:else if name === 'chevron'}
			<path d="M6 9l6 6 6-6" />
		{:else if name === 'gear'}
			<circle cx="12" cy="12" r="3" /><path
				d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"
			/>
		{:else if name === 'phone'}
			<rect x="6" y="2" width="12" height="20" rx="2.5" /><path d="M11 18h2" />
		{:else if name === 'tag'}
			<path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8zM7.5 7.5h.01" />
		{/if}
	</svg>
{/snippet}

<svelte:window onkeydown={sheet || openMenu ? onKeydown : undefined} onclick={onWindowClick} />

{#if isLoginPage}
	{@render children()}
{:else if authChecked}
	<div class="app">
		<nav class="topbar">
			<a class="nav-brand" href="/">Fangorn</a>
			<div class="nav-links">
				<a href="/" class:active={isActive('/')}>Dashboard</a>
				{#each GROUPS as group (group.id)}
					<div class="menu-wrap" data-menu onfocusout={onMenuFocusout}>
						<button
							class="menu-trigger"
							class:active={groupActive(group)}
							aria-haspopup="menu"
							aria-expanded={openMenu === group.id}
							aria-controls="menu-{group.id}"
							onclick={() => toggleMenu(group.id)}
							onkeydown={(e) => onTriggerKeydown(e, group.id)}
						>
							{group.label}{@render glyph('chevron')}
						</button>
						{#if openMenu === group.id}
							<div
								class="menu"
								id="menu-{group.id}"
								role="menu"
								tabindex="-1"
								aria-label={group.label}
								onkeydown={(e) => onMenuKeydown(e, group.id)}
							>
								{#each group.items as item (item.href)}
									<a
										role="menuitem"
										href={item.href}
										class:active={isActive(item.href)}
										aria-current={isActive(item.href) ? 'page' : undefined}
									>
										{@render glyph(item.icon)}{item.label}
									</a>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			<!-- Everything that adds to the ledger, in one place; the camera opens
			     straight from its menu item. -->
			<div class="menu-wrap add-wrap" data-menu onfocusout={onMenuFocusout}>
				<button
					class="add-trigger"
					aria-haspopup="menu"
					aria-expanded={openMenu === 'add'}
					aria-controls="menu-add"
					onclick={() => toggleMenu('add')}
					onkeydown={(e) => onTriggerKeydown(e, 'add')}
				>
					{@render glyph('plus')}<span>{capture.stage ? 'Reading…' : 'Add'}</span>{@render glyph('chevron')}
				</button>
				{#if openMenu === 'add'}
					<div
						class="menu right"
						id="menu-add"
						role="menu"
						tabindex="-1"
						aria-label="Add"
						onkeydown={(e) => onMenuKeydown(e, 'add')}
					>
						<a role="menuitem" href="/add">{@render glyph('list')}Log a transaction</a>
						<button role="menuitem" onclick={snapFromMenu} disabled={capture.stage !== null}>
							{@render glyph('camera')}Scan a receipt
						</button>
						<a role="menuitem" href="/transfers?new">{@render glyph('swap')}Record a transfer</a>
					</div>
				{/if}
			</div>

			<!-- On a phone the camera is its own button; the tab bar has the rest. -->
			<button
				class="camera"
				onclick={openCamera}
				disabled={capture.stage !== null}
				aria-label="Snap a receipt"
			>
				{@render glyph('camera')}<span>{capture.stage ? 'Reading…' : 'Receipt'}</span>
			</button>
			<a
				class="settings"
				class:active={isActive('/settings') || isActive('/shortcut')}
				href="/settings"
				aria-label="Settings"
				title="Settings"
			>
				{@render glyph('gear')}
			</a>
		</nav>

		<input
			use:registerCamera
			class="camera-input"
			type="file"
			accept="image/*"
			capture="environment"
			tabindex="-1"
			aria-hidden="true"
			onchange={onPhoto}
		/>

		{#if capture.stage || capture.notice}
			<div class="capture-banner {capture.notice?.tone ?? 'busy'}" role="status" aria-live="polite">
				{#if capture.stage}
					<span class="spinner" aria-hidden="true"></span>
					<span>{capture.stage === 'preparing' ? 'Preparing photo…' : 'Reading receipt…'}</span>
				{:else if capture.notice}
					<span class="banner-text">{capture.notice.text}</span>
					{#if capture.notice.review}
						<a href="/receipts" onclick={dismissNotice}>Review</a>
					{/if}
					<button class="banner-close" onclick={dismissNotice} aria-label="Dismiss">×</button>
				{/if}
			</div>
		{/if}

		<main>
			{@render children()}
		</main>

		<nav class="tabbar" aria-label="Main">
			<a href="/" class:active={isActive('/')}>{@render glyph('home')}<span>Home</span></a>
			<a href="/transactions" class:active={isActive('/transactions')}>
				{@render glyph('list')}<span>Activity</span>
			</a>
			<!-- Straight to the quick-log screen: logging is the reason to open the app. -->
			<a href="/add" class="add" class:active={isActive('/add')} aria-label="Log a transaction">
				{@render glyph('plus')}
			</a>
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
			<div class="sheet" role="dialog" aria-modal="true" aria-label="More">
				{#each MORE as item (item.href)}
					<a class="sheet-item" href={item.href} class:active={isActive(item.href)}>
						<span class="sheet-icon">{@render glyph(item.icon)}</span>
						<span class="sheet-text"><span>{item.label}</span></span>
					</a>
				{/each}
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
		align-items: center;
		gap: 0.25rem;
	}

	.nav-links > a,
	.menu-trigger {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		height: 36px;
		padding: 0 0.75rem;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		font: inherit;
		font-size: 0.9rem;
		font-weight: 500;
		color: rgba(255, 255, 255, 0.7);
		text-decoration: none;
		cursor: pointer;
		transition:
			color 0.15s,
			background 0.15s;
	}

	.nav-links > a:hover,
	.menu-trigger:hover,
	.menu-trigger[aria-expanded='true'] {
		color: white;
		background: rgba(255, 255, 255, 0.08);
	}

	.nav-links > a.active,
	.menu-trigger.active {
		color: var(--accent);
	}

	.menu-trigger :global(svg),
	.add-trigger :global(svg:last-child) {
		width: 14px;
		height: 14px;
		transition: transform 0.15s;
	}

	.menu-trigger[aria-expanded='true'] :global(svg),
	.add-trigger[aria-expanded='true'] :global(svg:last-child) {
		transform: rotate(180deg);
	}

	.menu-wrap {
		position: relative;
	}

	.menu {
		position: absolute;
		top: calc(100% + 6px);
		left: 0;
		z-index: 120;
		display: flex;
		flex-direction: column;
		min-width: 210px;
		padding: 0.375rem;
		border-radius: var(--radius);
		background: var(--surface);
		box-shadow:
			0 10px 30px rgba(0, 0, 0, 0.18),
			0 0 0 1px rgba(0, 0, 0, 0.04);
		animation: drop 0.12s ease-out;
	}

	.menu.right {
		left: auto;
		right: 0;
	}

	.menu [role='menuitem'] {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		min-height: 40px;
		padding: 0 0.75rem;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		font: inherit;
		font-size: 0.9rem;
		font-weight: 500;
		color: var(--ink);
		text-align: left;
		text-decoration: none;
		cursor: pointer;
	}

	.menu [role='menuitem'] :global(svg) {
		width: 18px;
		height: 18px;
		color: var(--muted-light);
	}

	.menu [role='menuitem']:hover,
	.menu [role='menuitem']:focus-visible {
		background: var(--bg);
		outline: none;
	}

	.menu [role='menuitem'].active {
		color: var(--accent-hover);
	}

	.menu [role='menuitem'].active :global(svg) {
		color: var(--accent-hover);
	}

	.menu [role='menuitem']:disabled {
		opacity: 0.5;
		cursor: progress;
	}

	.add-wrap {
		margin-left: auto;
	}

	.add-trigger {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		height: 36px;
		padding: 0 0.75rem 0 0.625rem;
		border: none;
		border-radius: 999px;
		background: var(--accent);
		font: inherit;
		font-size: 0.875rem;
		font-weight: 700;
		color: var(--ink);
		cursor: pointer;
	}

	.add-trigger:hover,
	.add-trigger[aria-expanded='true'] {
		background: var(--accent-hover);
	}

	.add-trigger :global(svg:first-child) {
		width: 18px;
		height: 18px;
		stroke-width: 2.5;
	}

	@keyframes drop {
		from {
			opacity: 0;
			transform: translateY(-4px);
		}
	}

	.camera {
		display: none;
		margin-left: auto;
		align-items: center;
		gap: 0.375rem;
		min-height: 36px;
		padding: 0 0.75rem;
		border: 1px solid rgba(255, 255, 255, 0.25);
		border-radius: 999px;
		background: none;
		font: inherit;
		font-size: 0.85rem;
		font-weight: 600;
		color: white;
		cursor: pointer;
	}

	.camera svg {
		width: 18px;
		height: 18px;
	}

	.camera:hover:not(:disabled) {
		border-color: var(--accent);
		color: var(--accent);
	}

	.camera:disabled {
		opacity: 0.7;
		cursor: progress;
	}

	.camera-input {
		display: none;
	}

	.capture-banner {
		position: fixed;
		right: 1.5rem;
		bottom: 1.5rem;
		z-index: 160;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		max-width: 420px;
		padding: 0.75rem 0.75rem 0.75rem 1rem;
		border-radius: var(--radius-sm);
		background: var(--ink);
		color: white;
		font-size: 0.875rem;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
		animation: rise 0.18s ease-out;
	}

	.capture-banner.ok {
		border-left: 4px solid var(--pos);
	}

	.capture-banner.warn {
		border-left: 4px solid var(--warn);
	}

	.capture-banner.error {
		border-left: 4px solid var(--neg);
	}

	.banner-text {
		flex: 1;
	}

	.capture-banner a {
		color: var(--accent);
		font-weight: 600;
	}

	.banner-close {
		width: 32px;
		height: 32px;
		border: none;
		background: none;
		font-size: 1.25rem;
		line-height: 1;
		color: rgba(255, 255, 255, 0.7);
		cursor: pointer;
	}

	.spinner {
		width: 16px;
		height: 16px;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.settings {
		display: grid;
		place-items: center;
		width: 36px;
		height: 36px;
		margin-left: 0.5rem;
		border-radius: 50%;
		color: rgba(255, 255, 255, 0.7);
	}

	.settings svg {
		width: 20px;
		height: 20px;
	}

	.settings:hover,
	.settings.active {
		color: var(--accent);
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

		/* On a phone Settings is in More, and the tab bar's + and the camera
		   button replace the Add menu. */
		.nav-links,
		.settings,
		.add-wrap {
			display: none;
		}

		.nav-brand {
			font-size: 1.125rem;
		}

		.camera {
			display: inline-flex;
			min-height: 36px;
			background: var(--accent);
			border-color: var(--accent);
			color: var(--ink);
		}

		.capture-banner {
			left: max(0.75rem, env(safe-area-inset-left));
			right: max(0.75rem, env(safe-area-inset-right));
			bottom: calc(64px + env(safe-area-inset-bottom) + 0.75rem);
			max-width: none;
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

		.tabbar .active:not(.add) {
			color: var(--ink);
		}

		.tabbar .active:not(.add) svg {
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
		}

		.tabbar .add svg {
			width: 28px;
			height: 28px;
			stroke-width: 2.5;
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

		.sheet-item.active .sheet-icon {
			color: var(--accent-hover);
		}

		.sheet-text {
			display: flex;
			flex-direction: column;
			line-height: 1.3;
		}
	}

	@keyframes rise {
		from {
			transform: translateY(12px);
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sheet,
		.menu,
		.capture-banner {
			animation: none;
		}

		.spinner {
			animation-duration: 2s;
		}
	}
</style>
