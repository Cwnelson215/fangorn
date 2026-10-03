<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getAccounts, getTransfers } from '$lib/api';
	import type { Account, Transfer } from '$lib/types';
	import { formatCurrency, formatDate } from '$lib/format';
	import TransferModal from '$lib/components/TransferModal.svelte';
	import Button from '$lib/components/Button.svelte';

	let transfers: Transfer[] = $state([]);
	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	let modalOpen = $state(false);
	let editing = $state<Transfer | null>(null);

	let ready = $state(false);

	onMount(async () => {
		await load();
		ready = true;
	});

	// The quick-log screen's Transfer button lands here as ?new.
	$effect(() => {
		if (!ready || !page.url.searchParams.has('new')) return;
		if (accounts.length >= 2) openCreate();
		goto('/transfers', { replaceState: true, noScroll: true, keepFocus: true });
	});

	async function load() {
		loading = true;
		loadError = null;
		try {
			[transfers, accounts] = await Promise.all([getTransfers(), getAccounts()]);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load transfers';
		} finally {
			loading = false;
		}
	}

	// Paying a card or loan from an account that isn't one, as the ledger marks it.
	function isDebtPayment(transfer: Transfer): boolean {
		const cls = (id: number) => accounts.find((a) => a.id === id)?.class;
		return cls(transfer.to_account_id) === 'liability' && cls(transfer.from_account_id) !== 'liability';
	}

	function openCreate() {
		editing = null;
		modalOpen = true;
	}

	function openEdit(transfer: Transfer) {
		editing = transfer;
		modalOpen = true;
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Transfers</h1>
			<p class="muted">Moving money between your own accounts — not counted as income or spending.</p>
		</div>
		<Button onclick={openCreate} disabled={accounts.length < 2}>Record Transfer</Button>
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if loadError}
		<p class="error-text">{loadError}</p>
	{:else if accounts.length < 2}
		<div class="card empty">
			<h2>You need at least two accounts</h2>
			<p class="muted">A transfer moves money from one account to another.</p>
			<a class="cta" href="/accounts">Go to accounts</a>
		</div>
	{:else if transfers.length === 0}
		<div class="card empty">
			<h2>No transfers yet</h2>
			<p class="muted">Record one when you move money between your accounts.</p>
		</div>
	{:else}
		<div class="card">
			<div class="table-scroll">
				<div class="list">
					{#each transfers as transfer (transfer.group_id)}
						<button class="row" onclick={() => openEdit(transfer)}>
							<span class="date">{formatDate(transfer.date)}</span>
							<span class="route">
								<span class="desc">{transfer.description}</span>
								<span class="accounts">
									{transfer.from_account} <span class="arrow">→</span>
									{transfer.to_account}
									{#if isDebtPayment(transfer)}· debt payment{/if}
								</span>
							</span>
							<span class="amount">{formatCurrency(transfer.amount)}</span>
						</button>
					{/each}
				</div>
			</div>
		</div>
	{/if}
</div>

<TransferModal {accounts} transfer={editing} bind:open={modalOpen} onsaved={load} />

<style>
	h1 {
		font-size: 1.5rem;
	}

	h2 {
		font-size: 1rem;
		margin-bottom: 0.5rem;
	}

	.table-scroll {
		overflow-x: auto;
	}

	.list {
		min-width: 480px;
		display: flex;
		flex-direction: column;
	}

	.row {
		display: grid;
		grid-template-columns: 120px 1fr 120px;
		gap: 1rem;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--line);
		background: none;
		border-left: none;
		border-right: none;
		border-top: none;
		font: inherit;
		text-align: left;
		cursor: pointer;
		width: 100%;
	}

	.row:last-child {
		border-bottom: none;
	}

	.row:hover {
		background: var(--surface-2);
	}

	.date {
		color: var(--muted-light);
		font-size: 0.85rem;
	}

	.route {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.desc {
		font-weight: 500;
	}

	.accounts {
		font-size: 0.75rem;
		color: var(--muted-light);
	}

	.arrow {
		color: var(--accent);
	}

	.amount {
		text-align: right;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	@media (max-width: 639px) {
		.row {
			grid-template-columns: minmax(0, 1fr) auto;
			grid-template-areas:
				'route amount'
				'route date';
			column-gap: 0.75rem;
			row-gap: 0;
			padding: 0.625rem 0.25rem;
			align-items: start;
		}

		.route {
			grid-area: route;
		}

		.amount {
			grid-area: amount;
		}

		.date {
			grid-area: date;
			text-align: right;
			font-size: 0.75rem;
		}

		.row:active {
			background: var(--bg);
		}
	}

	.cta {
		display: inline-block;
		background: var(--accent);
		color: var(--on-accent);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
		margin-top: 1rem;
	}
</style>
