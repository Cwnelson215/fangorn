<script lang="ts">
	import { createTransaction, deleteTransaction, receiptImageUrl, updateTransaction } from '$lib/api';
	import type { Account, Category, Transaction, TransactionInput } from '$lib/types';
	import { today } from '$lib/format';
	import { rememberLastAccount, startingAccount } from '$lib/remember';
	import AccountOptions from './AccountOptions.svelte';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	let {
		accounts,
		categories,
		transaction = null,
		open = $bindable(false),
		onsaved
	}: {
		accounts: Account[];
		categories: Category[];
		/** The income, expense or refund to change, or null to log a new one. */
		transaction?: Transaction | null;
		open?: boolean;
		onsaved: () => void;
	} = $props();

	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let kind = $state<'income' | 'expense' | 'refund'>('expense');
	let accountId = $state(0);
	let date = $state(today());
	let amount = $state('');
	let description = $state('');
	let merchant = $state('');
	let categoryId = $state(0);
	let notes = $state('');

	// Reset the form each time the modal opens, from the transaction if there is one.
	$effect(() => {
		if (!open) return;
		formError = null;
		if (transaction) {
			kind = transaction.kind as 'income' | 'expense' | 'refund';
			accountId = transaction.account_id;
			date = transaction.date;
			amount = String(Math.abs(transaction.amount));
			description = transaction.description;
			merchant = transaction.merchant ?? '';
			categoryId = transaction.category_id ?? 0;
			notes = transaction.notes ?? '';
		} else {
			kind = 'expense';
			accountId = startingAccount(accounts);
			date = today();
			amount = '';
			description = '';
			merchant = '';
			categoryId = 0;
			notes = '';
		}
	});

	// Only categories matching the selected direction are offered, so an expense
	// can't be filed under "Paycheck". A refund points back at what was spent, so
	// it picks from the same list an expense does.
	let categorySide = $derived(kind === 'refund' ? 'expense' : kind);
	let availableCategories = $derived(categories.filter((c) => c.kind === categorySide));

	function buildInput(): TransactionInput {
		return {
			account_id: accountId,
			date,
			// The server owns the sign; the form always sends a positive magnitude.
			amount: Math.abs(parseFloat(amount) || 0),
			kind,
			description: description.trim(),
			merchant: merchant.trim() || null,
			category_id: categoryId || null,
			notes: notes.trim() || null
		};
	}

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (!accountId || !amount || !description.trim()) return;
		// The server rejects this too; catching it here names the missing piece
		// instead of bouncing the whole form back.
		if (kind === 'refund' && !categoryId) {
			formError = 'Pick the category the money is coming back from.';
			return;
		}

		saving = true;
		formError = null;
		try {
			if (transaction) {
				await updateTransaction(transaction.id, buildInput());
			} else {
				await createTransaction(buildInput());
				// Only a new entry sets the default; fixing an old one shouldn't.
				rememberLastAccount(accountId);
			}
			open = false;
			onsaved();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the transaction';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!transaction) return;
		deleting = true;
		formError = null;
		try {
			await deleteTransaction(transaction.id);
			open = false;
			onsaved();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete the transaction';
		} finally {
			deleting = false;
		}
	}

	// Switching direction can strand a category from the other list.
	function onKindChange() {
		if (!availableCategories.some((c) => c.id === categoryId)) categoryId = 0;
	}
</script>

<Modal bind:open title={transaction ? 'Edit Transaction' : 'Log Transaction'}>
	<form onsubmit={handleSubmit}>
		<div class="toggle">
			<button
				type="button"
				class:active={kind === 'expense'}
				onclick={() => {
					kind = 'expense';
					onKindChange();
				}}
			>
				Money out
			</button>
			<button
				type="button"
				class:active={kind === 'income'}
				onclick={() => {
					kind = 'income';
					onKindChange();
				}}
			>
				Money in
			</button>
			<button
				type="button"
				class:active={kind === 'refund'}
				onclick={() => {
					kind = 'refund';
					onKindChange();
				}}
			>
				Refund
			</button>
		</div>

		{#if kind === 'refund'}
			<p class="hint muted">
				Money coming back from something you already logged. It lands in the account like income,
				but comes off that category's spending instead of counting as earnings.
			</p>
		{/if}

		<div class="form-row">
			<Field label="Amount" id="amount">
				<input
					id="amount"
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
			<Field label="Date" id="date">
				<input id="date" type="date" bind:value={date} disabled={saving} required />
			</Field>
		</div>

		<Field label="Description" id="description">
			<input
				id="description"
				bind:value={description}
				placeholder={kind === 'expense' ? 'Groceries' : kind === 'refund' ? 'Returned groceries' : 'Paycheck'}
				disabled={saving}
				required
			/>
		</Field>

		<div class="form-row">
			<Field label="Account" id="account">
				<select id="account" bind:value={accountId} disabled={saving}>
					<AccountOptions {accounts} />
				</select>
			</Field>
			<Field label={kind === 'refund' ? 'Refund of' : 'Category'} id="category">
				<select id="category" bind:value={categoryId} disabled={saving} required={kind === 'refund'}>
					<option value={0}>{kind === 'refund' ? 'Pick a category' : 'Uncategorized'}</option>
					{#each availableCategories as category (category.id)}
						<option value={category.id}>{category.name}</option>
					{/each}
				</select>
			</Field>
		</div>

		<Field label="Merchant" id="merchant">
			<input id="merchant" bind:value={merchant} placeholder="Optional" disabled={saving} />
		</Field>

		<Field label="Notes" id="txnNotes">
			<textarea id="txnNotes" bind:value={notes} disabled={saving}></textarea>
		</Field>

		{#if transaction?.receipt_id}
			<p class="hint muted">
				Posted from a receipt ·
				<a href={receiptImageUrl(transaction.receipt_id)} target="_blank" rel="noopener">view the photo</a>.
				Deleting this transaction deletes the photo too.
			</p>
		{/if}

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			{#if transaction}
				<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
					{deleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (open = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !amount || !description.trim()}>
				{saving ? 'Saving…' : transaction ? 'Save Changes' : 'Log It'}
			</Button>
		</div>
	</form>
</Modal>

<style>
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

	.hint {
		margin: 0.75rem 0 0;
		font-size: 0.8rem;
		line-height: 1.4;
	}

	.toggle {
		display: flex;
		background: var(--bg);
		border-radius: var(--radius-sm);
		padding: 0.25rem;
		gap: 0.25rem;
	}

	.toggle button {
		flex: 1;
		padding: 0.5rem;
		border: none;
		background: none;
		border-radius: 6px;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--muted);
		cursor: pointer;
	}

	.toggle button.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow);
	}
</style>
