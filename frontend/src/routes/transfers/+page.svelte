<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { getAccounts, getTransfers, createTransfer, refreshTransfer, cancelTransfer } from '$lib/api';
	import type { Account, Transfer } from '$lib/types';
	import TransferStatus from '$lib/components/TransferStatus.svelte';

	let accounts: Account[] = $state([]);
	let transfers: Transfer[] = $state([]);
	let sourceId = $state(0);
	let destId = $state(0);
	let amount = $state('');
	let description = $state('');
	let loading = $state(false);
	let error: string | null = $state(null);
	let pollInterval: ReturnType<typeof setInterval> | null = $state(null);

	const activeTransfers = $derived(transfers.filter(t => t.status === 'processing' || t.status === 'pending'));
	const pastTransfers = $derived(transfers.filter(t => t.status !== 'processing' && t.status !== 'pending'));

	onMount(async () => {
		try {
			[accounts, transfers] = await Promise.all([getAccounts(), getTransfers()]);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		}
		startPolling();
	});

	onDestroy(() => {
		stopPolling();
	});

	function startPolling() {
		stopPolling();
		pollInterval = setInterval(async () => {
			if (activeTransfers.length === 0) return;
			try {
				transfers = await getTransfers();
			} catch { /* ignore polling errors */ }
		}, 10000);
	}

	function stopPolling() {
		if (pollInterval) {
			clearInterval(pollInterval);
			pollInterval = null;
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!sourceId || !destId || !amount) return;
		if (sourceId === destId) {
			error = 'Source and destination must be different';
			return;
		}

		loading = true;
		error = null;
		try {
			await createTransfer(sourceId, destId, parseFloat(amount), description || undefined);
			transfers = await getTransfers();
			amount = '';
			description = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Transfer failed';
		} finally {
			loading = false;
		}
	}

	async function handleRefresh(id: number) {
		try {
			const updated = await refreshTransfer(id);
			transfers = transfers.map(t => t.id === id ? updated : t);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Refresh failed';
		}
	}

	async function handleCancel(id: number) {
		try {
			await cancelTransfer(id);
			transfers = await getTransfers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Cancel failed';
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString('en-US', {
			month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit'
		});
	}

	function formatAmount(amt: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amt);
	}
</script>

<svelte:head>
	<title>Fangorn - Transfers</title>
</svelte:head>

<div class="transfers-page">
	<h1>Transfer Money</h1>

	<div class="transfer-form-card">
		<form onsubmit={handleSubmit}>
			<div class="form-row">
				<div class="field">
					<label for="source">From</label>
					<select id="source" bind:value={sourceId} disabled={loading}>
						<option value={0}>Select source account</option>
						{#each accounts as account}
							<option value={account.id}>
								{account.name} {account.mask ? `(${account.mask})` : ''} — {formatAmount(account.current_balance ?? 0)}
							</option>
						{/each}
					</select>
				</div>
				<div class="arrow">→</div>
				<div class="field">
					<label for="dest">To</label>
					<select id="dest" bind:value={destId} disabled={loading}>
						<option value={0}>Select destination account</option>
						{#each accounts as account}
							<option value={account.id}>
								{account.name} {account.mask ? `(${account.mask})` : ''}
							</option>
						{/each}
					</select>
				</div>
			</div>
			<div class="form-row">
				<div class="field">
					<label for="amount">Amount</label>
					<input id="amount" type="number" step="0.01" min="0.01" placeholder="0.00" bind:value={amount} disabled={loading} />
				</div>
				<div class="field">
					<label for="desc">Description (optional)</label>
					<input id="desc" type="text" maxlength="15" placeholder="e.g. Rent" bind:value={description} disabled={loading} />
				</div>
			</div>
			<button type="submit" disabled={loading || !sourceId || !destId || !amount}>
				{loading ? 'Processing...' : 'Transfer'}
			</button>
		</form>

		{#if error}
			<p class="error">{error}</p>
		{/if}
	</div>

	{#if activeTransfers.length > 0}
		<h2>Active Transfers</h2>
		{#each activeTransfers as transfer}
			<div class="transfer-card active">
				<div class="transfer-header">
					<div class="transfer-info">
						<span class="transfer-amount">{formatAmount(transfer.amount)}</span>
						<span class="transfer-accounts">{transfer.source_account_name} → {transfer.destination_account_name}</span>
					</div>
					<div class="transfer-actions">
						<button class="btn-small" onclick={() => handleRefresh(transfer.id)}>Refresh</button>
						<button class="btn-small btn-danger" onclick={() => handleCancel(transfer.id)}>Cancel</button>
					</div>
				</div>
				<TransferStatus
					debitStatus={transfer.debit_status}
					creditStatus={transfer.credit_status}
					overallStatus={transfer.status}
				/>
				<div class="transfer-meta">
					{#if transfer.description}
						<span>{transfer.description}</span>
					{/if}
					<span>{formatDate(transfer.created_at)}</span>
				</div>
			</div>
		{/each}
	{/if}

	{#if pastTransfers.length > 0}
		<h2>Transfer History</h2>
		{#each pastTransfers as transfer}
			<div class="transfer-card" class:completed={transfer.status === 'completed'} class:failed={transfer.status === 'failed'} class:cancelled={transfer.status === 'cancelled'}>
				<div class="transfer-header">
					<div class="transfer-info">
						<span class="transfer-amount">{formatAmount(transfer.amount)}</span>
						<span class="transfer-accounts">{transfer.source_account_name} → {transfer.destination_account_name}</span>
					</div>
					<span class="status-badge" class:completed={transfer.status === 'completed'} class:failed={transfer.status === 'failed'} class:cancelled={transfer.status === 'cancelled'}>
						{transfer.status}
					</span>
				</div>
				<div class="transfer-meta">
					{#if transfer.description}
						<span>{transfer.description}</span>
					{/if}
					<span>{formatDate(transfer.created_at)}</span>
					{#if transfer.failure_reason}
						<span class="failure-reason">{transfer.failure_reason}</span>
					{/if}
				</div>
			</div>
		{/each}
	{/if}

	{#if transfers.length === 0 && !error}
		<div class="empty">
			<p>No transfers yet. Use the form above to move money between your accounts.</p>
		</div>
	{/if}
</div>

<style>
	.transfers-page {
		max-width: 800px;
	}

	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin-bottom: 1.5rem;
	}

	h2 {
		font-size: 1.1rem;
		font-weight: 600;
		margin: 2rem 0 1rem;
		color: #666;
	}

	.transfer-form-card {
		background: white;
		border-radius: 16px;
		padding: 1.5rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	.form-row {
		display: flex;
		gap: 1rem;
		margin-bottom: 1rem;
		align-items: end;
	}

	.arrow {
		font-size: 1.5rem;
		color: #999;
		padding-bottom: 0.25rem;
	}

	.field {
		flex: 1;
	}

	label {
		display: block;
		font-size: 0.8rem;
		font-weight: 500;
		color: #666;
		margin-bottom: 0.375rem;
	}

	select, input[type="number"], input[type="text"] {
		width: 100%;
		padding: 0.625rem 0.75rem;
		border: 1px solid #ddd;
		border-radius: 8px;
		font-size: 0.9rem;
		outline: none;
		transition: border-color 0.2s;
	}

	select:focus, input:focus {
		border-color: #4ecca3;
	}

	button[type="submit"] {
		width: 100%;
		background: #4ecca3;
		color: #1a1a2e;
		border: none;
		padding: 0.75rem;
		border-radius: 10px;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.2s;
	}

	button[type="submit"]:hover { background: #3db88f; }
	button[type="submit"]:disabled { opacity: 0.6; cursor: not-allowed; }

	.error {
		color: #ef4444;
		margin-top: 0.75rem;
		font-size: 0.9rem;
	}

	.transfer-card {
		background: white;
		border-radius: 12px;
		padding: 1.25rem;
		margin-bottom: 0.75rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	.transfer-card.active {
		border-left: 3px solid #3b82f6;
	}

	.transfer-card.completed {
		border-left: 3px solid #4ecca3;
	}

	.transfer-card.failed {
		border-left: 3px solid #ef4444;
	}

	.transfer-card.cancelled {
		border-left: 3px solid #999;
	}

	.transfer-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.transfer-info {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.transfer-amount {
		font-size: 1.25rem;
		font-weight: 700;
	}

	.transfer-accounts {
		font-size: 0.85rem;
		color: #666;
	}

	.transfer-actions {
		display: flex;
		gap: 0.5rem;
	}

	.btn-small {
		padding: 0.375rem 0.75rem;
		border: 1px solid #ddd;
		border-radius: 6px;
		background: white;
		font-size: 0.8rem;
		cursor: pointer;
		transition: all 0.2s;
	}

	.btn-small:hover { background: #f3f4f6; }

	.btn-danger {
		color: #ef4444;
		border-color: #fecaca;
	}

	.btn-danger:hover { background: #fef2f2; }

	.status-badge {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.25rem 0.625rem;
		border-radius: 12px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.status-badge.completed { background: #dcfce7; color: #166534; }
	.status-badge.failed { background: #fef2f2; color: #991b1b; }
	.status-badge.cancelled { background: #f3f4f6; color: #6b7280; }

	.transfer-meta {
		display: flex;
		gap: 1rem;
		font-size: 0.8rem;
		color: #999;
		margin-top: 0.5rem;
	}

	.failure-reason {
		color: #ef4444;
	}

	.empty {
		text-align: center;
		padding: 3rem;
		color: #999;
	}
</style>
