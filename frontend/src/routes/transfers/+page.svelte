<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		createTransfer,
		deleteTransfer,
		getAccounts,
		getTransfers,
		updateTransfer
	} from '$lib/api';
	import type { Account, Transfer, TransferInput } from '$lib/types';
	import { formatCurrency, formatDate, today } from '$lib/format';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

	let transfers: Transfer[] = $state([]);
	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	let modalOpen = $state(false);
	let editing = $state<Transfer | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let fromId = $state(0);
	let toId = $state(0);
	let amount = $state('');
	let date = $state(today());
	let description = $state('');
	let notes = $state('');

	let ready = $state(false);

	onMount(async () => {
		await load();
		ready = true;
	});

	// Quick add from the tab bar lands here as ?new, possibly while this page
	// is already open.
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

	function openCreate() {
		editing = null;
		fromId = accounts[0]?.id ?? 0;
		toId = accounts[1]?.id ?? 0;
		amount = '';
		date = today();
		description = '';
		notes = '';
		formError = null;
		modalOpen = true;
	}

	function openEdit(transfer: Transfer) {
		editing = transfer;
		fromId = transfer.from_account_id;
		toId = transfer.to_account_id;
		amount = String(transfer.amount);
		date = transfer.date;
		description = transfer.description;
		notes = transfer.notes ?? '';
		formError = null;
		modalOpen = true;
	}

	function buildInput(): TransferInput {
		return {
			from_account_id: fromId,
			to_account_id: toId,
			amount: Math.abs(parseFloat(amount) || 0),
			date,
			description: description.trim() || 'Transfer',
			notes: notes.trim() || null
		};
	}

	let sameAccount = $derived(fromId !== 0 && fromId === toId);

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (!fromId || !toId || !amount) return;
		if (sameAccount) {
			formError = 'Pick two different accounts';
			return;
		}

		saving = true;
		formError = null;
		try {
			if (editing) {
				await updateTransfer(editing.group_id, buildInput());
			} else {
				await createTransfer(buildInput());
			}
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the transfer';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!editing) return;
		deleting = true;
		formError = null;
		try {
			await deleteTransfer(editing.group_id);
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete the transfer';
		} finally {
			deleting = false;
		}
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

<Modal bind:open={modalOpen} title={editing ? 'Edit Transfer' : 'Record Transfer'}>
	<form onsubmit={handleSubmit}>
		<div class="form-row">
			<Field label="From" id="from">
				<select id="from" bind:value={fromId} disabled={saving}>
					{#each accounts as account (account.id)}
						<option value={account.id}>{account.name}</option>
					{/each}
				</select>
			</Field>
			<Field label="To" id="to">
				<select id="to" bind:value={toId} disabled={saving}>
					{#each accounts as account (account.id)}
						<option value={account.id}>{account.name}</option>
					{/each}
				</select>
			</Field>
		</div>

		{#if sameAccount}
			<p class="error-text">Source and destination must be different accounts.</p>
		{/if}

		<div class="form-row">
			<Field label="Amount" id="transferAmount">
				<input
					id="transferAmount"
					type="number"
					inputmode="decimal"
					step="0.01"
					min="0.01"
					placeholder="0.00"
					bind:value={amount}
					disabled={saving}
					required
				/>
			</Field>
			<Field label="Date" id="transferDate">
				<input id="transferDate" type="date" bind:value={date} disabled={saving} required />
			</Field>
		</div>

		<Field label="Description" id="transferDesc">
			<input
				id="transferDesc"
				bind:value={description}
				placeholder="Transfer to savings"
				disabled={saving}
			/>
		</Field>

		<Field label="Notes" id="transferNotes">
			<textarea id="transferNotes" bind:value={notes} disabled={saving}></textarea>
		</Field>

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			{#if editing}
				<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
					{deleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (modalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !amount || sameAccount}>
				{saving ? 'Saving…' : editing ? 'Save Changes' : 'Record It'}
			</Button>
		</div>
	</form>
</Modal>

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
		border-bottom: 1px solid #f0f0f0;
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
		background: #fafafa;
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

	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.form-row {
		display: flex;
		gap: 1rem;
	}

	.form-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.spacer {
		flex: 1;
	}

	.cta {
		display: inline-block;
		background: var(--accent);
		color: var(--ink);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
		margin-top: 1rem;
	}
</style>
