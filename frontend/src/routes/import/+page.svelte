<script lang="ts">
	import { getSupportedBanks, importCSV, getAccounts } from '$lib/api';
	import type { Account, ImportResult } from '$lib/types';
	import { onMount } from 'svelte';

	let banks: string[] = $state([]);
	let accounts: Account[] = $state([]);
	let selectedBank = $state('');
	let selectedAccountId: number | undefined = $state(undefined);
	let file: File | null = $state(null);
	let loading = $state(false);
	let error: string | null = $state(null);
	let result: ImportResult | null = $state(null);

	onMount(async () => {
		try {
			[banks, accounts] = await Promise.all([getSupportedBanks(), getAccounts()]);
			if (banks.length > 0) selectedBank = banks[0];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		}
	});

	function onFileChange(e: Event) {
		const input = e.target as HTMLInputElement;
		file = input.files?.[0] ?? null;
		result = null;
		error = null;
	}

	async function handleSubmit() {
		if (!file || !selectedBank) return;
		loading = true;
		error = null;
		result = null;
		try {
			result = await importCSV(file, selectedBank, selectedAccountId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Import failed';
		} finally {
			loading = false;
		}
	}

	function formatBankName(name: string): string {
		return name.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
	}
</script>

<svelte:head>
	<title>Fangorn - Import CSV</title>
</svelte:head>

<div class="import-page">
	<div class="import-card">
		<h1>Import Transactions from CSV</h1>
		<p>Upload a CSV file exported from your bank to import transactions.</p>

		<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
			<div class="field">
				<label for="bank">Bank Format</label>
				<select id="bank" bind:value={selectedBank}>
					{#each banks as bank}
						<option value={bank}>{formatBankName(bank)}</option>
					{/each}
				</select>
			</div>

			<div class="field">
				<label for="account">Account (optional)</label>
				<select id="account" bind:value={selectedAccountId}>
					<option value={undefined}>Create new account</option>
					{#each accounts as account}
						<option value={account.id}>{account.name}{account.mask ? ` (***${account.mask})` : ''}</option>
					{/each}
				</select>
			</div>

			<div class="field">
				<label for="file">CSV File</label>
				<input id="file" type="file" accept=".csv" onchange={onFileChange} />
			</div>

			<button type="submit" disabled={loading || !file}>
				{loading ? 'Importing...' : 'Import'}
			</button>
		</form>

		{#if result}
			<div class="success-message">
				Imported {result.imported} transactions, skipped {result.skipped} duplicates.
			</div>
		{/if}

		{#if error}
			<p class="error">{error}</p>
		{/if}
	</div>
</div>

<style>
	.import-page {
		display: flex;
		justify-content: center;
		padding-top: 3rem;
	}

	.import-card {
		background: white;
		border-radius: 16px;
		padding: 3rem;
		max-width: 520px;
		width: 100%;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin-bottom: 0.75rem;
	}

	p {
		color: #666;
		margin-bottom: 2rem;
		line-height: 1.6;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	label {
		font-size: 0.875rem;
		font-weight: 600;
		color: #333;
	}

	select, input[type="file"] {
		padding: 0.625rem 0.75rem;
		border: 1px solid #ddd;
		border-radius: 8px;
		font-size: 0.9rem;
		background: white;
	}

	select:focus {
		outline: none;
		border-color: #4ecca3;
	}

	button {
		background: #4ecca3;
		color: #1a1a2e;
		border: none;
		padding: 0.875rem 2.5rem;
		border-radius: 10px;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.2s;
		margin-top: 0.5rem;
	}

	button:hover {
		background: #3db88f;
	}

	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.success-message {
		background: #dcfce7;
		color: #166534;
		padding: 1rem;
		border-radius: 8px;
		font-weight: 500;
		margin-top: 1.5rem;
	}

	.error {
		color: #ef4444;
		margin-top: 1rem;
	}
</style>
