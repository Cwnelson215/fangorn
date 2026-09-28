<!--
	Closing an account: move out whatever it still holds, then delete it.

	Removing the remaining balance records where the money went, as a transfer to
	(or, for a debt or an overdraft, from) another account, so net worth doesn't
	drop by money that only moved. Choosing no account drops it instead — an
	adjustment that lowers net worth without counting as spending — after a
	second confirmation. Deleting removes the account with every
	transaction, trade and recurring rule on it; the other side of any transfer
	stays on its own account, so that account's balance doesn't change. Because
	it can't be undone, it asks for the account's institution to be typed, or its
	name when no institution was recorded.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { createTransfer, deleteAccount, dropBalance, getAccounts } from '$lib/api';
	import type { Account } from '$lib/types';
	import { formatCurrency, today } from '$lib/format';
	import { confirmationPhrase, matchesConfirmation } from '$lib/closeAccount';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Modal from './Modal.svelte';
	import AccountOptions from './AccountOptions.svelte';

	let {
		account,
		holdingsValue = 0,
		onchanged
	}: {
		account: Account;
		/** Market value of any securities held; they can't be moved as cash. */
		holdingsValue?: number;
		onchanged: () => void;
	} = $props();

	let cash = $derived(Math.round(account.cash_balance * 100) / 100);
	let isLiability = $derived(account.class === 'liability');
	// Money sits in the account (move it out), or is owed on it (pay it from elsewhere).
	let outgoing = $derived(cash > 0);

	// --- Remove remaining balance -------------------------------------------
	let moveOpen = $state(false);
	let others = $state<Account[]>([]);
	// An account to move it to (or pay it from), or 'drop' for none.
	let target = $state<number | 'drop'>('drop');
	let moveDate = $state(today());
	let moving = $state(false);
	let moveError = $state<string | null>(null);

	async function openMove() {
		moveError = null;
		moveDate = today();
		moveOpen = true;
		try {
			others = (await getAccounts()).filter((a) => a.id !== account.id);
			target = others[0]?.id ?? 'drop';
		} catch (e) {
			moveError = e instanceof Error ? e.message : 'Could not load accounts';
		}
	}

	async function move(event: SubmitEvent) {
		event.preventDefault();
		if (target === 'drop') {
			// Not undoable from the form, so it gets its own confirmation.
			moveOpen = false;
			dropError = null;
			dropOpen = true;
			return;
		}
		const targetId = target;
		moving = true;
		moveError = null;
		try {
			await createTransfer({
				from_account_id: outgoing ? account.id : targetId,
				to_account_id: outgoing ? targetId : account.id,
				amount: Math.abs(cash),
				date: moveDate,
				description: `Remaining balance of ${account.name}`,
				notes: null
			});
			moveOpen = false;
			onchanged();
		} catch (e) {
			moveError = e instanceof Error ? e.message : 'Could not move the balance';
		} finally {
			moving = false;
		}
	}

	// --- Drop it ----------------------------------------------------------------
	let dropOpen = $state(false);
	let dropping = $state(false);
	let dropError = $state<string | null>(null);

	async function drop() {
		dropping = true;
		dropError = null;
		try {
			await dropBalance(account.id, moveDate);
			dropOpen = false;
			onchanged();
		} catch (e) {
			dropError = e instanceof Error ? e.message : 'Could not drop the balance';
		} finally {
			dropping = false;
		}
	}

	// --- Delete ---------------------------------------------------------------
	let deleteOpen = $state(false);
	let typed = $state('');
	let deleting = $state(false);
	let deleteError = $state<string | null>(null);

	let phrase = $derived(confirmationPhrase(account));
	let confirmed = $derived(matchesConfirmation(typed, phrase));

	function openDelete() {
		typed = '';
		deleteError = null;
		deleteOpen = true;
	}

	async function remove(event: SubmitEvent) {
		event.preventDefault();
		if (!confirmed) return;
		deleting = true;
		deleteError = null;
		try {
			await deleteAccount(account.id);
			deleteOpen = false;
			goto('/accounts');
		} catch (e) {
			deleteError = e instanceof Error ? e.message : 'Could not delete the account';
		} finally {
			deleting = false;
		}
	}
</script>

<div class="card">
	<h2>Close this account</h2>
	<p class="muted">
		{#if cash === 0}
			The {holdingsValue > 0 ? 'cash' : 'balance'} is $0.
		{:else if isLiability && cash < 0}
			{formatCurrency(-cash)} is still owed. Record how it was paid off before deleting, or your net
			worth will jump by that amount.
		{:else if cash < 0}
			It's overdrawn by {formatCurrency(-cash)}. Record where the money came from before deleting.
		{:else}
			It still holds {formatCurrency(cash)}. Record where it went before deleting, or your net worth
			will drop by that amount.
		{/if}
		{#if holdingsValue > 0}
			Its investments ({formatCurrency(holdingsValue)}) aren't cash; sell them with Log trade first.
		{/if}
	</p>
	<div class="actions">
		<Button variant="secondary" size="sm" disabled={cash === 0} onclick={openMove}>
			Remove remaining balance
		</Button>
		<Button variant="danger" size="sm" onclick={openDelete}>Delete account</Button>
	</div>
</div>

<Modal bind:open={moveOpen} title="Remove remaining balance">
	<form onsubmit={move}>
		<p>
			{#if outgoing}
				Move <strong>{formatCurrency(cash)}</strong> out of {account.name} to:
			{:else}
				Pay the <strong>{formatCurrency(-cash)}</strong>
				{isLiability ? 'owed on' : 'overdraft on'}
				{account.name} from:
			{/if}
		</p>
		<div class="form-row">
			<Field label={outgoing ? 'To account' : 'From account'} id="close-target">
				<select id="close-target" bind:value={target}>
					<AccountOptions accounts={others} />
					<option value="drop">No account — drop it</option>
				</select>
			</Field>
			<Field label="Date" id="close-date">
				<input id="close-date" type="date" bind:value={moveDate} required />
			</Field>
		</div>
		<p class="muted">
			{#if target === 'drop'}
				Nothing is recorded as going anywhere: your net worth {outgoing ? 'drops' : 'rises'} by
				{formatCurrency(Math.abs(cash))}. It doesn't count as spending{outgoing ? '' : ' or income'}.
			{:else}
				This records a transfer, so your net worth stays the same.
			{/if}
		</p>
		{#if moveError}
			<p class="error-text">{moveError}</p>
		{/if}
		<div class="form-actions">
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (moveOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={moving}>
				{moving ? 'Recording…' : target === 'drop' ? 'Drop balance…' : 'Record transfer'}
			</Button>
		</div>
	</form>
</Modal>

<Modal bind:open={dropOpen} title="Drop {formatCurrency(Math.abs(cash))}?">
	<div class="confirm">
		<p>
			{#if outgoing}
				The {formatCurrency(cash)} left in {account.name} will be taken off your books without going to
				another account. Your net worth drops by that much.
			{:else}
				The {formatCurrency(-cash)}
				{isLiability ? 'owed on' : 'overdraft on'}
				{account.name} will be written off without being paid from another account. Your net worth rises
				by that much.
			{/if}
			It won't show up as spending, income or in any budget.
		</p>
		{#if dropError}
			<p class="error-text">{dropError}</p>
		{/if}
		<div class="form-actions">
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (dropOpen = false)}>Cancel</Button>
			<Button variant="danger" disabled={dropping} onclick={drop}>
				{dropping ? 'Dropping…' : 'Drop it'}
			</Button>
		</div>
	</div>
</Modal>

<Modal bind:open={deleteOpen} title="Delete {account.name}?">
	<form onsubmit={remove}>
		<p>
			This permanently deletes the account and every transaction, trade, receipt and recurring rule on
			it. It can't be undone. Transfers to and from other accounts stay on those accounts.
		</p>
		{#if cash !== 0 || holdingsValue > 0}
			<p class="warn-text">
				It isn't empty: your net worth will change by {formatCurrency(Math.abs(cash) + holdingsValue)}.
				{#if cash !== 0}Use Remove remaining balance first to move it or drop it.{/if}
			</p>
		{/if}
		<Field
			label={account.institution_name?.trim()
				? `Type the institution, ${phrase}, to confirm`
				: `Type the account name, ${phrase}, to confirm`}
			id="close-confirm"
		>
			<input
				id="close-confirm"
				bind:value={typed}
				autocomplete="off"
				autocapitalize="off"
				spellcheck="false"
			/>
		</Field>
		{#if deleteError}
			<p class="error-text">{deleteError}</p>
		{/if}
		<div class="form-actions">
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (deleteOpen = false)}>Cancel</Button>
			<Button variant="danger" type="submit" disabled={!confirmed || deleting}>
				{deleting ? 'Deleting…' : 'Delete forever'}
			</Button>
		</div>
	</form>
</Modal>

<style>
	h2 {
		margin-bottom: 0.5rem;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 1rem;
	}

	form,
	.confirm {
		display: flex;
		flex-direction: column;
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
