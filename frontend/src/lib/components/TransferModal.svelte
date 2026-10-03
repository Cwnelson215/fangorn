<script lang="ts">
	import { createTransfer, deleteTransfer, updateTransfer } from '$lib/api';
	import type { Account, Transfer, TransferInput } from '$lib/types';
	import { today } from '$lib/format';
	import { pickRemembered, rememberId } from '$lib/remember';
	import AccountOptions from './AccountOptions.svelte';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	let {
		accounts,
		transfer = null,
		open = $bindable(false),
		onsaved
	}: {
		accounts: Account[];
		/** The transfer being edited, or null to record a new one. */
		transfer?: Transfer | null;
		open?: boolean;
		onsaved: () => void;
	} = $props();

	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let fromId = $state(0);
	let toId = $state(0);
	let amount = $state('');
	let date = $state(today());
	let description = $state('');
	let notes = $state('');

	// Reset the form each time the modal opens, from the transfer if editing.
	$effect(() => {
		if (!open) return;
		formError = null;
		if (transfer) {
			fromId = transfer.from_account_id;
			toId = transfer.to_account_id;
			amount = String(transfer.amount);
			date = transfer.date;
			description = transfer.description;
			notes = transfer.notes ?? '';
		} else {
			fromId = pickRemembered('transfer.from', accounts, accounts[0]?.id ?? 0);
			toId = pickRemembered('transfer.to', accounts, accounts[1]?.id ?? 0);
			// The remembered pair can collide once one side has been changed alone.
			if (toId === fromId) toId = accounts.find((a) => a.id !== fromId)?.id ?? 0;
			amount = '';
			date = today();
			description = '';
			notes = '';
		}
	});

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
			if (transfer) {
				await updateTransfer(transfer.group_id, buildInput());
			} else {
				await createTransfer(buildInput());
				rememberId('transfer.from', fromId);
				rememberId('transfer.to', toId);
			}
			open = false;
			onsaved();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the transfer';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!transfer) return;
		deleting = true;
		formError = null;
		try {
			await deleteTransfer(transfer.group_id);
			open = false;
			onsaved();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete the transfer';
		} finally {
			deleting = false;
		}
	}
</script>

<Modal bind:open title={transfer ? 'Edit Transfer' : 'Record Transfer'}>
	<form onsubmit={handleSubmit}>
		<div class="form-row">
			<Field label="From" id="from">
				<select id="from" bind:value={fromId} disabled={saving}>
					<AccountOptions {accounts} />
				</select>
			</Field>
			<Field label="To" id="to">
				<select id="to" bind:value={toId} disabled={saving}>
					<AccountOptions {accounts} />
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
			{#if transfer}
				<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
					{deleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (open = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !amount || sameAccount}>
				{saving ? 'Saving…' : transfer ? 'Save Changes' : 'Record It'}
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
</style>
