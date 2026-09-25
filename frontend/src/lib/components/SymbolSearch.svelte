<script lang="ts">
	// Ticker input with search-as-you-type. Typing a symbol you already know
	// ("FZROX") works without picking from the list; the list is there for when
	// you only remember "fidelity international".
	import { searchSecurities } from '$lib/api';
	import type { SecurityMatch } from '$lib/types';

	let {
		id,
		value = $bindable(''),
		disabled = false,
		onselect
	}: {
		id: string;
		value?: string;
		disabled?: boolean;
		onselect?: (match: SecurityMatch) => void;
	} = $props();

	const TYPE_LABELS: Record<string, string> = {
		MUTUALFUND: 'Fund',
		ETF: 'ETF',
		EQUITY: 'Stock',
		MONEYMARKET: 'Money market',
		CRYPTOCURRENCY: 'Crypto'
	};

	let matches = $state<SecurityMatch[]>([]);
	let open = $state(false);
	let active = $state(-1);
	let searchError = $state(false);

	let timer: ReturnType<typeof setTimeout> | undefined;
	let controller: AbortController | undefined;

	function onInput() {
		clearTimeout(timer);
		const q = value.trim();
		if (q.length < 2) {
			matches = [];
			open = false;
			return;
		}
		timer = setTimeout(() => runSearch(q), 250);
	}

	async function runSearch(q: string) {
		// Abort the previous request so a slow response can't overwrite a newer one.
		controller?.abort();
		controller = new AbortController();
		try {
			matches = await searchSecurities(q, controller.signal);
			searchError = false;
			active = -1;
			open = matches.length > 0;
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') return;
			// Search is a convenience; a typed symbol still works without it.
			searchError = true;
			matches = [];
			open = false;
		}
	}

	function choose(match: SecurityMatch) {
		value = match.symbol;
		open = false;
		matches = [];
		onselect?.(match);
	}

	function onKeydown(event: KeyboardEvent) {
		if (!open) return;
		if (event.key === 'ArrowDown') {
			event.preventDefault();
			active = (active + 1) % matches.length;
		} else if (event.key === 'ArrowUp') {
			event.preventDefault();
			active = active <= 0 ? matches.length - 1 : active - 1;
		} else if (event.key === 'Enter' && active >= 0) {
			event.preventDefault();
			choose(matches[active]);
		} else if (event.key === 'Escape') {
			// Close the list without letting the modal's Escape handler close the dialog.
			event.stopPropagation();
			open = false;
		}
	}
</script>

<div class="symbol-search">
	<input
		{id}
		bind:value
		{disabled}
		oninput={onInput}
		onkeydown={onKeydown}
		onblur={() => setTimeout(() => (open = false), 150)}
		placeholder="FZROX, VOO, or a fund name"
		autocomplete="off"
		spellcheck="false"
		role="combobox"
		aria-expanded={open}
		aria-controls="{id}-results"
		aria-autocomplete="list"
		required
	/>
	{#if open}
		<ul class="results" id="{id}-results" role="listbox">
			{#each matches as match, i (match.symbol)}
				<li role="option" aria-selected={i === active}>
					<button
						type="button"
						class:active={i === active}
						onmousedown={(e) => e.preventDefault()}
						onclick={() => choose(match)}
					>
						<span class="symbol">{match.symbol}</span>
						<span class="name">{match.name}</span>
						<span class="meta">
							{TYPE_LABELS[match.quote_type] ?? match.quote_type}{match.exchange
								? ` · ${match.exchange}`
								: ''}
						</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
	{#if searchError}
		<span class="search-note">Search isn't available right now; type the ticker symbol.</span>
	{/if}
</div>

<style>
	.symbol-search {
		position: relative;
	}

	input {
		text-transform: uppercase;
	}

	input::placeholder {
		text-transform: none;
	}

	.results {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		z-index: 10;
		list-style: none;
		margin: 0;
		padding: 0.25rem;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		border: 1px solid var(--border);
		box-shadow: var(--shadow-lg);
		max-height: 18rem;
		overflow-y: auto;
	}

	.results button {
		display: grid;
		grid-template-columns: 5.5rem 1fr;
		column-gap: 0.5rem;
		width: 100%;
		padding: 0.5rem;
		border: none;
		border-radius: 6px;
		background: none;
		font: inherit;
		text-align: left;
		cursor: pointer;
		color: var(--ink);
	}

	.results button:hover,
	.results button.active {
		background: var(--bg);
	}

	.symbol {
		font-weight: 700;
		font-size: 0.875rem;
	}

	.name {
		font-size: 0.875rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.meta {
		grid-column: 2;
		font-size: 0.75rem;
		color: var(--muted-light);
	}

	.search-note {
		display: block;
		margin-top: 0.375rem;
		font-size: 0.75rem;
		color: var(--muted-light);
	}
</style>
