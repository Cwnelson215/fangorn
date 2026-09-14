<script lang="ts">
	import { onMount } from 'svelte';
	import {
		archiveAccount,
		createAccount,
		getAccounts,
		unarchiveAccount,
		updateAccount
	} from '$lib/api';
	import type { Account, AccountInput, AccountType } from '$lib/types';
	import { ACCOUNT_TYPE_LABELS } from '$lib/types';
	import { formatCurrency, today } from '$lib/format';
	import AccountCard from '$lib/components/AccountCard.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

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

	function openCreate() {
		editing = null;
		name = '';
		institution = '';
		type = 'checking';
		mask = '';
		startingBalance = '';
		startingDate = today();
		notes = '';
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
			notes: notes.trim() || null
		};
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

		{#each [{ label: 'Assets', items: assets }, { label: 'Credit Cards & Loans', items: liabilities }] as group}
			{#if group.items.length > 0}
				<section>
					<h2>{group.label}</h2>
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
			{/if}
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
				<input id="institution" bind:value={institution} placeholder="Gesa" disabled={saving} />
			</Field>
		</div>

		<div class="form-row">
			<Field
				label={isLiabilityType
					? 'Amount currently owed'
					: type === 'investment'
						? 'Cash balance'
						: 'Starting balance'}
				id="balance"
				hint={isLiabilityType
					? 'Enter what you owe as a positive number'
					: type === 'investment'
						? 'Uninvested cash only (including a money market core position). Add what you hold from the account page.'
						: undefined}
			>
				<input
					id="balance"
					type="number"
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
		font-size: 1rem;
		margin-bottom: 0.75rem;
		color: var(--muted);
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

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
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
