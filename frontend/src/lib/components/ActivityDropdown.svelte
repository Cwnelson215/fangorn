<script lang="ts" module>
	/** One line of what moved a budget, goal or debt. `amount` is signed. */
	export interface ActivityRow {
		key: string | number;
		date: string;
		description: string;
		detail?: string;
		amount: number;
		/** Planned, not happened yet: shown muted. */
		pending?: boolean;
	}
</script>

<script lang="ts">
	// The transactions behind a line on the budgets page, closed until asked for.
	// Rows are fetched when it's first opened, and again whenever `stamp` changes
	// while it is open — the page bumps that each time it reloads the month.
	import { untrack } from 'svelte';
	import { formatDateShort, formatSigned } from '$lib/format';

	let {
		load,
		stamp,
		label = 'Transactions',
		empty = 'Nothing yet this month.'
	}: {
		load: () => Promise<ActivityRow[]>;
		stamp: unknown;
		label?: string;
		empty?: string;
	} = $props();

	let open = $state(false);
	let rows = $state<ActivityRow[] | null>(null);
	let error = $state<string | null>(null);
	let request = 0;

	$effect(() => {
		void stamp;
		if (!open) return;
		const mine = ++request;
		untrack(load)
			.then((loaded) => {
				if (mine !== request) return; // a later reload won
				rows = loaded;
				error = null;
			})
			.catch((e) => {
				if (mine !== request) return;
				error = e instanceof Error ? e.message : 'Could not load the transactions';
			});
	});
</script>

<details bind:open>
	<summary>{label}{#if open && rows}&nbsp;({rows.length}){/if}</summary>
	{#if error}
		<p class="error-text">{error}</p>
	{:else if rows === null}
		<p class="muted note">Loading…</p>
	{:else if rows.length === 0}
		<p class="muted note">{empty}</p>
	{:else}
		<ul>
			{#each rows as row (row.key)}
				<li class:pending={row.pending}>
					<span class="date">{formatDateShort(row.date)}</span>
					<span class="desc">
						<span class="name">{row.description}</span>
						{#if row.detail}<span class="detail">{row.detail}</span>{/if}
					</span>
					<span class="amount" class:pos={row.amount > 0} class:neg={row.amount < 0}>
						{formatSigned(row.amount)}
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</details>

<style>
	details {
		margin-top: 0.375rem;
		font-size: 0.8125rem;
	}

	summary {
		display: inline-block;
		cursor: pointer;
		color: var(--muted);
		padding: 0.25rem 0;
		user-select: none;
		list-style: none;
	}

	summary::-webkit-details-marker {
		display: none;
	}

	summary::before {
		content: '▸';
		display: inline-block;
		width: 1em;
	}

	details[open] > summary::before {
		content: '▾';
	}

	summary:hover,
	summary:focus-visible {
		color: var(--ink);
	}

	.note {
		margin: 0.25rem 0 0 1em;
	}

	ul {
		list-style: none;
		margin: 0.25rem 0 0;
		padding: 0.25rem 0.75rem;
		background: var(--bg);
		border-radius: var(--radius-sm);
	}

	li {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.375rem 0;
		border-bottom: 1px solid var(--line);
	}

	li:last-child {
		border-bottom: none;
	}

	.date {
		flex: none;
		width: 3.5rem;
		color: var(--muted-light);
	}

	.desc {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	.name,
	.detail {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.detail {
		font-size: 0.75rem;
		color: var(--muted-light);
	}

	.amount {
		flex: none;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.pos {
		color: var(--pos);
	}

	/* Planned, not happened: quieter than what has. */
	.pending .name,
	.pending .amount {
		color: var(--muted);
		font-weight: 500;
		font-style: italic;
	}
</style>
