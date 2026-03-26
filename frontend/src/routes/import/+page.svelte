<script lang="ts">
	import { getSupportedBanks, importCSV, getAccounts, detectCSVHeaders, saveBankFormat } from '$lib/api';
	import type { Account, ImportResult, DetectResult } from '$lib/types';
	import { onMount } from 'svelte';

	let banks: string[] = $state([]);
	let accounts: Account[] = $state([]);
	let selectedBank = $state('');
	let selectedAccountId: number | undefined = $state(undefined);
	let file: File | null = $state(null);
	let loading = $state(false);
	let error: string | null = $state(null);
	let result: ImportResult | null = $state(null);

	// New bank format state
	let addingNewBank = $state(false);
	let newBankName = $state('');
	let detectResult: DetectResult | null = $state(null);
	let dateColumn = $state('');
	let amountColumn = $state('');
	let descriptionColumn = $state('');
	let categoryColumn = $state('');
	let negateAmounts = $state(true);
	let savingFormat = $state(false);

	onMount(async () => {
		try {
			[banks, accounts] = await Promise.all([getSupportedBanks(), getAccounts()]);
			if (banks.length > 0) {
				selectedBank = banks[0];
			} else {
				enterAddNewBank();
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		}
	});

	function onFileChange(e: Event) {
		const input = e.target as HTMLInputElement;
		file = input.files?.[0] ?? null;
		result = null;
		error = null;
		detectResult = null;
	}

	function enterAddNewBank() {
		addingNewBank = true;
		selectedBank = '';
		detectResult = null;
		file = null;
		newBankName = '';
		dateColumn = '';
		amountColumn = '';
		descriptionColumn = '';
		categoryColumn = '';
		negateAmounts = true;
	}


	async function handleDetect() {
		if (!file) return;
		loading = true;
		error = null;
		try {
			detectResult = await detectCSVHeaders(file);
			if (detectResult.headers.length > 0) {
				dateColumn = detectResult.headers[0];
				amountColumn = detectResult.headers[0];
				descriptionColumn = detectResult.headers[0];
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to detect CSV headers';
		} finally {
			loading = false;
		}
	}

	async function handleSaveFormat() {
		if (!newBankName || !dateColumn || !amountColumn || !descriptionColumn) return;
		savingFormat = true;
		error = null;
		try {
			await saveBankFormat({
				bank_name: newBankName.toLowerCase().replace(/\s+/g, '_'),
				date_column: dateColumn,
				amount_column: amountColumn,
				description_column: descriptionColumn,
				category_column: categoryColumn || undefined,
				negate_amounts: negateAmounts
			});
			const bankKey = newBankName.toLowerCase().replace(/\s+/g, '_');
			banks = [...banks, bankKey].sort();
			selectedBank = bankKey;
			addingNewBank = false;
			detectResult = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save bank format';
		} finally {
			savingFormat = false;
		}
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
			{#if banks.length > 0 && !addingNewBank}
				<div class="field">
					<label for="bank">Bank Format</label>
					<select id="bank" bind:value={selectedBank}>
						{#each banks as bank}
							<option value={bank}>{formatBankName(bank)}</option>
						{/each}
					</select>
				</div>
			{/if}

			{#if !addingNewBank}
				<button type="button" class="add-bank-btn" onclick={enterAddNewBank}>
					+ Add New Bank Format
				</button>
			{/if}

			{#if addingNewBank}
				<div class="new-bank-section">
					<div class="section-header">
						<h2>New Bank Format</h2>
						{#if banks.length > 0}
							<button type="button" class="cancel-btn" onclick={() => { addingNewBank = false; if (banks.length > 0) selectedBank = banks[0]; }}>
								Cancel
							</button>
						{/if}
					</div>
					<div class="field">
						<label for="file-detect">CSV File</label>
						<input id="file-detect" type="file" accept=".csv" onchange={onFileChange} />
					</div>

					{#if file && !detectResult}
						<button type="button" onclick={handleDetect} disabled={loading}>
							{loading ? 'Detecting...' : 'Detect Columns'}
						</button>
					{/if}

					{#if detectResult}
						<div class="field">
							<label for="new-bank-name">Bank Name</label>
							<input id="new-bank-name" type="text" bind:value={newBankName} placeholder="e.g. Gesa Credit Union" />
						</div>

						<div class="field">
							<label for="date-col">Date Column</label>
							<select id="date-col" bind:value={dateColumn}>
								{#each detectResult.headers as h}
									<option value={h}>{h}</option>
								{/each}
							</select>
						</div>

						<div class="field">
							<label for="amount-col">Amount Column</label>
							<select id="amount-col" bind:value={amountColumn}>
								{#each detectResult.headers as h}
									<option value={h}>{h}</option>
								{/each}
							</select>
						</div>

						<div class="field">
							<label for="desc-col">Description Column</label>
							<select id="desc-col" bind:value={descriptionColumn}>
								{#each detectResult.headers as h}
									<option value={h}>{h}</option>
								{/each}
							</select>
						</div>

						<div class="field">
							<label for="cat-col">Category Column (optional)</label>
							<select id="cat-col" bind:value={categoryColumn}>
								<option value="">None</option>
								{#each detectResult.headers as h}
									<option value={h}>{h}</option>
								{/each}
							</select>
						</div>

						<div class="field checkbox-field">
							<label>
								<input type="checkbox" bind:checked={negateAmounts} />
								Negate amounts (negative in CSV = expense in app)
							</label>
						</div>

						{#if detectResult.preview_rows.length > 0}
							<div class="preview">
								<h3>Preview</h3>
								<div class="preview-table-wrap">
									<table>
										<thead>
											<tr>
												{#each detectResult.headers as h}
													<th>{h}</th>
												{/each}
											</tr>
										</thead>
										<tbody>
											{#each detectResult.preview_rows as row}
												<tr>
													{#each row as cell}
														<td>{cell}</td>
													{/each}
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}

						<button type="button" onclick={handleSaveFormat} disabled={savingFormat || !newBankName}>
							{savingFormat ? 'Saving...' : 'Save Bank Format & Import'}
						</button>
					{/if}
				</div>
			{:else}
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
			{/if}
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
		max-width: 640px;
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

	.checkbox-field {
		flex-direction: row;
		align-items: center;
	}

	.checkbox-field label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
	}

	label {
		font-size: 0.875rem;
		font-weight: 600;
		color: #333;
	}

	select, input[type="file"], input[type="text"] {
		padding: 0.625rem 0.75rem;
		border: 1px solid #ddd;
		border-radius: 8px;
		font-size: 0.9rem;
		background: white;
	}

	select:focus, input[type="text"]:focus {
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

	.add-bank-btn {
		background: transparent;
		color: #4ecca3;
		border: 2px dashed #4ecca3;
		padding: 0.75rem 1.5rem;
		border-radius: 10px;
		font-size: 0.95rem;
		font-weight: 600;
		cursor: pointer;
		transition: all 0.2s;
	}

	.add-bank-btn:hover {
		background: #f0fdf4;
		border-color: #3db88f;
		color: #3db88f;
	}

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.section-header h2 {
		font-size: 1.1rem;
		font-weight: 600;
		color: #333;
		margin: 0;
	}

	.cancel-btn {
		background: transparent;
		color: #666;
		border: 1px solid #ddd;
		padding: 0.375rem 1rem;
		border-radius: 8px;
		font-size: 0.8rem;
		font-weight: 500;
		cursor: pointer;
		margin-top: 0;
	}

	.cancel-btn:hover {
		background: #f3f4f6;
		color: #333;
	}

	.new-bank-section {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		border: 1px solid #e5e7eb;
		border-radius: 12px;
		padding: 1.5rem;
		background: #fafafa;
	}

	.preview {
		margin-top: 0.5rem;
	}

	.preview h3 {
		font-size: 0.875rem;
		font-weight: 600;
		margin-bottom: 0.5rem;
		color: #333;
	}

	.preview-table-wrap {
		overflow-x: auto;
		border-radius: 8px;
		border: 1px solid #e5e7eb;
	}

	.preview table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8rem;
	}

	.preview th, .preview td {
		padding: 0.5rem 0.75rem;
		text-align: left;
		border-bottom: 1px solid #e5e7eb;
		white-space: nowrap;
	}

	.preview th {
		background: #f3f4f6;
		font-weight: 600;
		color: #374151;
	}

	.preview tbody tr:last-child td {
		border-bottom: none;
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
