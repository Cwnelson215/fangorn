<script lang="ts">
	import { onMount } from 'svelte';
	import {
		archiveAccount,
		createAccount,
		getAccounts,
		unarchiveAccount,
		updateAccount
	} from '$lib/api';
	import type { Account, AccountInput, AccountType, TaxTreatment } from '$lib/types';
	import { ACCOUNT_TYPE_LABELS, TAX_TREATMENT_LABELS, holdsSecurities } from '$lib/types';
	import { formatCurrency, today } from '$lib/format';
	import AccountCard from '$lib/components/AccountCard.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';
	import GroupBySwitch from '$lib/components/GroupBySwitch.svelte';
	import { groupAccounts, institutionNames } from '$lib/grouping';
	import { grouping } from '$lib/grouping.svelte';
	import { defaultCashFund } from '$lib/cashfund';

	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let showArchived = $state(false);

	// Form state
	let modalOpen = $state(false);
	let editing = $state<Account | null>(null);
	let saving = $state(false);
	let formError = $state<string | null>(null);

	let name = $state('');
	let institution = $state('');
	let type = $state<AccountType>('checking');
	let mask = $state('');
	let startingBalance = $state('');
	let startingDate = $state(today());
	let notes = $state('');
	let taxTreatment = $state<TaxTreatment>('roth');
	let apy = $state<string | number>('');
	// Where an investment account's cash sits. It follows the institution
	// (Fidelity → SPAXX) until someone types in it.
	let cashFund = $state('');
	let cashFundTouched = $state(false);

	onMount(load);

	async function load() {
		loading = true;
		loadError = null;
		try {
			accounts = await getAccounts(showArchived);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load accounts';
		} finally {
			loading = false;
		}
	}

	let assets = $derived(accounts.filter((a) => a.class === 'asset'));
	let liabilities = $derived(accounts.filter((a) => a.class === 'liability'));
	let totalAssets = $derived(assets.reduce((sum, a) => sum + a.balance, 0));
	let totalDebt = $derived(liabilities.reduce((sum, a) => sum + Math.abs(a.balance), 0));
	let groups = $derived(groupAccounts(accounts, grouping.by));
	let institutions = $derived(institutionNames(accounts));

	function openCreate() {
		editing = null;
		name = '';
		institution = '';
		type = 'checking';
		mask = '';
		startingBalance = '';
		startingDate = today();
		notes = '';
		taxTreatment = 'roth';
		apy = '';
		cashFund = '';
		cashFundTouched = false;
		formError = null;
		modalOpen = true;
	}

	function openEdit(account: Account) {
		editing = account;
		name = account.name;
		institution = account.institution_name ?? '';
		type = account.type;
		mask = account.mask ?? '';
		startingBalance = String(account.starting_balance);
		startingDate = account.starting_balance_date;
		notes = account.notes ?? '';
		taxTreatment = account.tax_treatment ?? 'roth';
		// An account added before cash funds existed gets its institution's default
		// offered here, so saving it once links the fund.
		cashFund = account.cash_fund ?? defaultCashFund(account.institution_name) ?? '';
		cashFundTouched = account.cash_fund != null;
		formError = null;
		modalOpen = true;
	}

	// Liabilities are stored as negative balances, but asking someone to type
	// "-400" for a card they owe $400 on is a trap. The form takes the amount owed
	// and flips the sign here.
	let isLiabilityType = $derived(type === 'credit_card' || type === 'loan');

	function buildInput(): AccountInput {
		const raw = Math.abs(parseFloat(startingBalance) || 0);
		return {
			name: name.trim(),
			institution_name: institution.trim() || null,
			type,
			mask: mask.trim() || null,
			starting_balance: isLiabilityType ? -raw : raw,
			starting_balance_date: startingDate,
			currency: 'USD',
			color: null,
			notes: notes.trim() || null,
			tax_treatment: type === 'retirement' ? taxTreatment : null,
			// Only the opening rate; later changes go into the account's rate history.
			apy: !editing && type === 'high_yield_savings' ? Number(apy) : null,
			cash_fund: holdsSecurities(type) && cashFund.trim() ? cashFund.trim().toUpperCase() : null
		};
	}

	function onInstitutionInput() {
		if (!cashFundTouched) cashFund = defaultCashFund(institution) ?? '';
	}

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (!name.trim()) return;

		saving = true;
		formError = null;
		try {
			if (editing) {
				await updateAccount(editing.id, buildInput());
			} else {
				await createAccount(buildInput());
			}
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the account';
		} finally {
			saving = false;
		}
	}

	async function toggleArchive(account: Account) {
		try {
			if (account.archived) {
				await unarchiveAccount(account.id);
			} else {
				await archiveAccount(account.id);
			}
			await load();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not update the account';
		}
	}

	async function toggleShowArchived() {
		showArchived = !showArchived;
		await load();
	}
</script>

<div class="page">
	<div class="page-header">
		<h1>Accounts</h1>
		<div class="actions">
			<Button variant="ghost" size="sm" onclick={toggleShowArchived}>
				{showArchived ? 'Hide archived' : 'Show archived'}
			</Button>
			<Button onclick={openCreate}>Add Account</Button>
		</div>
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if loadError}
		<p class="error-text">{loadError}</p>
	{:else if accounts.length === 0}
		<div class="card empty">
			<h2>No accounts yet</h2>
			<p class="muted">
				Add each account with the balance it holds today. Everything you log afterwards adjusts it
				from that starting point.
			</p>
		</div>
	{:else}
		<div class="totals">
			<div class="card stat">
				<span class="stat-label">Assets</span>
				<span class="stat-value">{formatCurrency(totalAssets)}</span>
			</div>
			<div class="card stat">
				<span class="stat-label">Owed</span>
				<span class="stat-value neg">{formatCurrency(totalDebt)}</span>
			</div>
			<div class="card stat">
				<span class="stat-label">Net Worth</span>
				<span class="stat-value">{formatCurrency(totalAssets - totalDebt)}</span>
			</div>
		</div>

		<div class="group-bar">
			<span class="muted">Group by</span>
			<GroupBySwitch />
		</div>

		{#each groups as group (group.key)}
			<section>
				<h2>
					<span>{group.label}</span>
					<span class="group-total" class:neg={group.total < 0}>{formatCurrency(group.total)}</span>
				</h2>
				<div class="grid">
					{#each group.items as account (account.id)}
						<div class="account-wrapper">
							<AccountCard {account} />
							<div class="row-actions">
								<Button variant="ghost" size="sm" onclick={() => openEdit(account)}>Edit</Button>
								<Button variant="ghost" size="sm" onclick={() => toggleArchive(account)}>
									{account.archived ? 'Restore' : 'Archive'}
								</Button>
							</div>
						</div>
					{/each}
				</div>
			</section>
		{/each}
	{/if}
</div>

<Modal bind:open={modalOpen} title={editing ? 'Edit Account' : 'Add Account'}>
	<form onsubmit={handleSubmit}>
		<Field label="Account name" id="name">
			<input id="name" bind:value={name} placeholder="Gesa Checking" disabled={saving} required />
		</Field>

		<div class="form-row">
			<Field label="Type" id="type">
				<select id="type" bind:value={type} disabled={saving}>
					{#each Object.entries(ACCOUNT_TYPE_LABELS) as [value, label]}
						<option {value}>{label}</option>
					{/each}
				</select>
			</Field>
			<Field label="Institution" id="institution">
				<input
					id="institution"
					list="institution-names"
					bind:value={institution}
					oninput={onInstitutionInput}
					placeholder="Gesa"
					autocomplete="off"
					disabled={saving}
				/>
				<datalist id="institution-names">
					{#each institutions as i (i)}
						<option value={i}></option>
					{/each}
				</datalist>
			</Field>
		</div>

		{#if type === 'retirement'}
			<Field
				label="Tax treatment"
				id="taxTreatment"
				hint={taxTreatment === 'roth'
					? 'Contributed after tax; withdrawals in retirement are tax-free.'
					: 'Contributed pre-tax; withdrawals are taxed as income.'}
			>
				<select id="taxTreatment" bind:value={taxTreatment} disabled={saving}>
					{#each Object.entries(TAX_TREATMENT_LABELS) as [value, label]}
						<option {value}>{label}</option>
					{/each}
				</select>
			</Field>
		{/if}

		{#if holdsSecurities(type)}
			<Field
				label="Cash sits in"
				id="cashFund"
				hint={editing && !editing.cash_fund
					? "The money market fund holding this account's cash. Its yield is looked up daily and pays a dividend each month, counted from today."
					: "The money market fund holding this account's cash (SPAXX at Fidelity). Its yield is looked up daily and pays a dividend each month. Leave blank if it doesn't earn."}
			>
				<input
					id="cashFund"
					bind:value={cashFund}
					oninput={() => (cashFundTouched = true)}
					placeholder="SPAXX"
					autocapitalize="characters"
					autocomplete="off"
					disabled={saving}
				/>
			</Field>
		{/if}

		{#if type === 'high_yield_savings'}
			{#if editing}
				<p class="muted rate-note">
					Interest rate: {editing.apy != null ? `${editing.apy}% APY` : 'not set'}. Change it from
					the account's page, so months already earned keep their rate.
				</p>
			{:else}
				<Field
					label="Interest rate (APY %)"
					id="apy"
					hint="Each month's interest is posted automatically once the month ends, from the as-of date below."
				>
					<input
						id="apy"
						type="number"
						inputmode="decimal"
						step="0.001"
						min="0"
						max="99.999"
						placeholder="4.35"
						bind:value={apy}
						disabled={saving}
						required
					/>
				</Field>
			{/if}
		{/if}

		<div class="form-row">
			<Field
				label={isLiabilityType
					? 'Amount currently owed'
					: holdsSecurities(type)
						? 'Cash balance'
						: 'Starting balance'}
				id="balance"
				hint={isLiabilityType
					? 'Enter what you owe as a positive number'
					: holdsSecurities(type)
						? 'Uninvested cash only (including a money market core position). Add what you hold from the account page.'
						: undefined}
			>
				<input
					id="balance"
					type="number"
					inputmode="decimal"
					step="0.01"
					min="0"
					placeholder="0.00"
					bind:value={startingBalance}
					disabled={saving}
				/>
			</Field>
			<Field label="As of" id="startingDate" hint="The date that balance was accurate">
				<input id="startingDate" type="date" bind:value={startingDate} disabled={saving} required />
			</Field>
		</div>

		<div class="form-row">
			<Field label="Last 4 digits" id="mask">
				<input id="mask" bind:value={mask} placeholder="1234" maxlength="4" disabled={saving} />
			</Field>
		</div>

		<Field label="Notes" id="notes">
			<textarea id="notes" bind:value={notes} disabled={saving}></textarea>
		</Field>

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			<Button variant="secondary" onclick={() => (modalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || !name.trim()}>
				{saving ? 'Saving…' : editing ? 'Save Changes' : 'Add Account'}
			</Button>
		</div>
	</form>
</Modal>

<style>
	h2 {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		font-size: 1rem;
		margin-bottom: 0.75rem;
		color: var(--muted);
	}

	.group-total {
		font-variant-numeric: tabular-nums;
		font-weight: 600;
	}

	.group-total.neg {
		color: var(--neg);
	}

	.rate-note {
		margin: 0;
		font-size: 0.8125rem;
	}

	.group-bar {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.5rem;
		font-size: 0.8125rem;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.totals {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 1rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
	}

	.stat-label {
		font-size: 0.8125rem;
		color: var(--muted);
		font-weight: 500;
	}

	.stat-value {
		font-size: 1.5rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	@media (max-width: 639px) {
		.totals {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 0.5rem;
		}

		.totals .stat:last-child {
			grid-column: 1 / -1;
			order: -1;
		}

		.totals .stat:not(:last-child) {
			padding: 0.75rem;
		}

		.totals .stat:not(:last-child) .stat-value {
			font-size: 1.125rem;
		}
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(260px, 100%), 1fr));
		gap: 1rem;
	}

	.account-wrapper {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.row-actions {
		display: flex;
		gap: 0.25rem;
		padding-left: 0.25rem;
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
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}
</style>
