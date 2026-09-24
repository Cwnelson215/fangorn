<script lang="ts">
	import { deleteReceipt, postReceipt, receiptImageUrl, retryReceipt } from '$lib/api';
	import { formatCurrency } from '$lib/format';
	import { today } from '$lib/format';
	import { reasonText } from '$lib/receipts';
	import type { Account, Category, Receipt } from '$lib/types';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	let {
		receipt,
		open = $bindable(false),
		accounts,
		categories,
		onchange
	}: {
		receipt: Receipt | null;
		open?: boolean;
		accounts: Account[];
		categories: Category[];
		/** Called after the receipt is posted, deleted or sent back to be read. */
		onchange: () => void;
	} = $props();

	let kind = $state<'expense' | 'refund'>('expense');
	let amount = $state('');
	let date = $state('');
	let description = $state('');
	let merchant = $state('');
	let accountId = $state(0);
	let categoryId = $state(0);
	let notes = $state('');
	let busy = $state(false);
	let formError = $state<string | null>(null);

	let expenseCategories = $derived(categories.filter((c) => c.kind === 'expense'));

	// Prefill from what was read each time the dialog opens on a receipt. What
	// was read is a starting point: every field stays editable.
	$effect(() => {
		if (!open || !receipt) return;
		const r = receipt;
		kind = r.review_reasons.includes('looks_like_return') ? 'refund' : 'expense';
		amount = r.total != null ? r.total.toFixed(2) : '';
		date = r.purchased_on ?? today();
		description = r.merchant ?? 'Receipt';
		merchant = r.merchant ?? '';
		accountId = r.account_id ?? accounts[0]?.id ?? 0;
		categoryId = r.category_id ?? 0;
		notes = '';
		formError = null;
	});

	async function act(fn: () => Promise<unknown>, failure: string) {
		busy = true;
		formError = null;
		try {
			await fn();
			open = false;
			onchange();
		} catch (e) {
			formError = e instanceof Error ? e.message : failure;
		} finally {
			busy = false;
		}
	}

	function handleSubmit(event: Event) {
		event.preventDefault();
		if (!receipt || !accountId || !amount || !description.trim()) return;
		if (kind === 'refund' && !categoryId) {
			formError = 'Pick the category the money is coming back from.';
			return;
		}
		const id = receipt.id;
		act(
			() =>
				postReceipt(id, {
					account_id: accountId,
					date,
					// The server owns the sign; the form always sends a positive magnitude.
					amount: Math.abs(parseFloat(amount) || 0),
					kind,
					description: description.trim(),
					merchant: merchant.trim() || null,
					category_id: categoryId || null,
					notes: notes.trim() || null
				}),
			'Could not post the receipt'
		);
	}
</script>

<Modal bind:open title="Review receipt">
	{#if receipt}
		{@const r = receipt}
		<div class="review">
			<a class="photo" href={receiptImageUrl(r.id)} target="_blank" rel="noopener">
				<img src={receiptImageUrl(r.id)} alt="Receipt" />
			</a>

			{#if r.review_reasons.length > 0}
				<ul class="reasons">
					{#each r.review_reasons as reason (reason)}
						<li>{reasonText(reason, r)}</li>
					{/each}
				</ul>
			{/if}

			{#if r.line_items && r.line_items.length > 0}
				<details class="items">
					<summary>{r.line_items.length} item{r.line_items.length === 1 ? '' : 's'} read</summary>
					<ul>
						{#each r.line_items as item, i (i)}
							<li>
								<span>{item.description}</span>
								<span class="num">{formatCurrency(item.amount)}</span>
							</li>
						{/each}
					</ul>
				</details>
			{/if}

			<form onsubmit={handleSubmit}>
				<div class="toggle">
					<button type="button" class:active={kind === 'expense'} onclick={() => (kind = 'expense')}>
						Purchase
					</button>
					<button type="button" class:active={kind === 'refund'} onclick={() => (kind = 'refund')}>
						Return / refund
					</button>
				</div>

				<div class="form-row">
					<Field label="Amount" id="rAmount">
						<input
							id="rAmount"
							type="number"
							inputmode="decimal"
							step="0.01"
							min="0.01"
							bind:value={amount}
							disabled={busy}
							required
						/>
					</Field>
					<Field label="Date" id="rDate">
						<input id="rDate" type="date" bind:value={date} disabled={busy} required />
					</Field>
				</div>

				<Field label="Description" id="rDescription">
					<input id="rDescription" bind:value={description} disabled={busy} required />
				</Field>

				<div class="form-row">
					<Field label="Account" id="rAccount">
						<select id="rAccount" bind:value={accountId} disabled={busy}>
							{#each accounts as account (account.id)}
								<option value={account.id}>
									{account.name}{account.mask ? ` ····${account.mask}` : ''}
								</option>
							{/each}
						</select>
					</Field>
					<Field label={kind === 'refund' ? 'Refund of' : 'Category'} id="rCategory">
						<select id="rCategory" bind:value={categoryId} disabled={busy} required={kind === 'refund'}>
							<option value={0}>{kind === 'refund' ? 'Pick a category' : 'Uncategorized'}</option>
							{#each expenseCategories as category (category.id)}
								<option value={category.id}>{category.name}</option>
							{/each}
						</select>
					</Field>
				</div>

				<Field label="Merchant" id="rMerchant">
					<input id="rMerchant" bind:value={merchant} placeholder="Optional" disabled={busy} />
				</Field>

				<Field label="Notes" id="rNotes">
					<textarea id="rNotes" bind:value={notes} disabled={busy}></textarea>
				</Field>

				{#if formError}
					<p class="error-text">{formError}</p>
				{/if}

				<div class="form-actions">
					<Button
						variant="danger"
						size="sm"
						disabled={busy}
						onclick={() => act(() => deleteReceipt(r.id), 'Could not delete the receipt')}
					>
						Delete
					</Button>
					<Button
						variant="ghost"
						size="sm"
						disabled={busy}
						onclick={() => act(() => retryReceipt(r.id), 'Could not send it back')}
					>
						Read again
					</Button>
					<span class="spacer"></span>
					<Button type="submit" disabled={busy || !amount || !description.trim() || !accountId}>
						{busy ? 'Saving…' : 'Post it'}
					</Button>
				</div>
			</form>
		</div>
	{/if}
</Modal>

<style>
	.review {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	/* 16px stops iOS Safari zooming the page when a field is focused. */
	.review :global(input),
	.review :global(select),
	.review :global(textarea) {
		font-size: 16px;
	}

	.photo {
		display: block;
		background: var(--bg);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}

	.photo img {
		display: block;
		width: 100%;
		max-height: 40vh;
		object-fit: contain;
	}

	.reasons {
		margin: 0;
		padding: 0.75rem 1rem 0.75rem 2rem;
		background: #fff8e6;
		border-radius: var(--radius-sm);
		color: #8a5a00;
		font-size: 0.875rem;
	}

	.reasons li + li {
		margin-top: 0.25rem;
	}

	.items {
		font-size: 0.875rem;
	}

	.items summary {
		cursor: pointer;
		color: var(--muted);
	}

	.items ul {
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
		max-height: 12rem;
		overflow-y: auto;
	}

	.items li {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.2rem 0;
		border-bottom: 1px solid var(--divider);
	}

	.num {
		font-variant-numeric: tabular-nums;
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
	}

	.spacer {
		flex: 1;
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
