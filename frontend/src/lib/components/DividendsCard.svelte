<script lang="ts">
	// Dividends and stock splits the app found on held stocks and funds, waiting
	// to be confirmed. The price service says a dividend is owed (shares held
	// into the ex-date × the amount per share) and, for a stock, the day it is
	// paid — but not what a reinvestment bought, and never whether the trades
	// were already entered in post-split shares. So nothing is applied by
	// itself. Confirm is the one tap; Edit is for a different amount or date, or
	// a reinvested dividend. The card renders nothing when there is nothing.
	import { onMount } from 'svelte';
	import {
		confirmDividend,
		confirmSplit,
		dismissDividend,
		dismissSplit,
		getDividends,
		getSplits
	} from '$lib/api';
	import type { Dividend, Split } from '$lib/types';
	import { formatCurrency, formatDate, formatPrice, formatShares, today } from '$lib/format';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	// Without accountId it lists every account's and names the account on each.
	let { accountId, onchange }: { accountId?: number; onchange?: () => void } = $props();

	let dividends = $state<Dividend[]>([]);
	let splits = $state<Split[]>([]);
	let error = $state<string | null>(null);
	// Which row is being saved: "d12" or "s3".
	let busy = $state<string | null>(null);

	let editing = $state<Dividend | null>(null);
	let editOpen = $state(false);
	let amount = $state<string | number>('');
	let date = $state(today());
	let reinvested = $state(false);
	let shares = $state<string | number>('');
	let formError = $state<string | null>(null);

	onMount(load);

	async function load() {
		try {
			[dividends, splits] = await Promise.all([getDividends(accountId), getSplits(accountId)]);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load dividends';
		}
	}

	let title = $derived(
		splits.length === 0
			? 'Dividends to confirm'
			: dividends.length === 0
				? 'Stock splits to confirm'
				: 'Dividends and splits to confirm'
	);

	// A mutual fund's dividend is almost always reinvested, and that needs the
	// share count from the statement, so its Confirm opens the form instead.
	function reinvests(d: Dividend): boolean {
		return d.quote_type === 'MUTUALFUND';
	}

	// Known to be paid on a day that hasn't come: nothing to confirm yet.
	function unpaid(d: Dividend): boolean {
		return d.pay_date != null && d.pay_date > today();
	}

	async function act(key: string, what: () => Promise<void>, failed: string): Promise<boolean> {
		busy = key;
		try {
			await what();
			error = null;
			await load();
			onchange?.();
			return true;
		} catch (e) {
			const message = e instanceof Error ? e.message : failed;
			if (editOpen) formError = message;
			else error = message;
			return false;
		} finally {
			busy = null;
		}
	}

	function confirm(d: Dividend) {
		if (reinvests(d)) return edit(d);
		act(`d${d.id}`, () => confirmDividend(d.id), 'Could not confirm the dividend');
	}

	function dismiss(d: Dividend) {
		act(`d${d.id}`, () => dismissDividend(d.id), 'Could not dismiss the dividend');
	}

	function edit(d: Dividend) {
		editing = d;
		amount = d.amount;
		date = d.pay_date != null && d.pay_date <= today() ? d.pay_date : today();
		reinvested = reinvests(d);
		// A starting point only: the statement has the real figure.
		shares = d.price > 0 ? Number((d.amount / d.price).toFixed(3)) : '';
		formError = null;
		editOpen = true;
	}

	async function save(event: Event) {
		event.preventDefault();
		if (!editing) return;
		const d = editing;
		formError = null;
		const ok = await act(
			`d${d.id}`,
			() =>
				confirmDividend(d.id, {
					amount: Number(amount),
					date,
					reinvested,
					shares: reinvested ? Number(shares) : undefined
				}),
			'Could not confirm the dividend'
		);
		if (ok) editOpen = false;
	}

	function ratio(s: Split): string {
		return `${s.numerator}-for-${s.denominator}`;
	}
</script>

{#if dividends.length > 0 || splits.length > 0 || error}
	<div class="card">
		<h2>{title}</h2>
		{#if error}
			<p class="error-text">{error}</p>
		{/if}
		<ul class="dividends">
			{#each splits as s (s.id)}
				<li>
					<span class="what">
						<span class="symbol">
							{s.symbol}
							{ratio(s)}
							{s.numerator < s.denominator ? 'reverse split' : 'split'}
						</span>
						<span class="muted sub">
							{#if accountId == null}{s.account_name} · {/if}{formatDate(s.split_date)} ·
							{formatShares(s.shares)} sh → {formatShares(s.shares_after)} sh
						</span>
					</span>
					<span class="actions">
						<Button
							size="sm"
							disabled={busy === `s${s.id}`}
							onclick={() => act(`s${s.id}`, () => confirmSplit(s.id), 'Could not apply the split')}
						>
							Apply
						</Button>
						<Button
							variant="ghost"
							size="sm"
							disabled={busy === `s${s.id}`}
							onclick={() => act(`s${s.id}`, () => dismissSplit(s.id), 'Could not dismiss the split')}
						>
							Dismiss
						</Button>
					</span>
				</li>
			{/each}
			{#each dividends as d (d.id)}
				<li>
					<span class="what">
						<span class="symbol">{d.symbol} dividend</span>
						<span class="muted sub">
							{#if accountId == null}{d.account_name} · {/if}{formatShares(d.shares)} sh × {formatPrice(
								d.per_share
							)} ·
							{#if d.pay_date}
								{unpaid(d) ? 'pays' : 'paid'} {formatDate(d.pay_date)}
							{:else}
								ex-date {formatDate(d.ex_date)}
							{/if}
						</span>
					</span>
					<span class="amount num">≈ {formatCurrency(d.amount)}</span>
					<span class="actions">
						{#if !unpaid(d)}
							<Button size="sm" disabled={busy === `d${d.id}`} onclick={() => confirm(d)}>
								{reinvests(d) ? 'Confirm…' : 'Confirm'}
							</Button>
							{#if !reinvests(d)}
								<Button variant="ghost" size="sm" disabled={busy === `d${d.id}`} onclick={() => edit(d)}>
									Edit
								</Button>
							{/if}
						{/if}
						<Button variant="ghost" size="sm" disabled={busy === `d${d.id}`} onclick={() => dismiss(d)}>
							Dismiss
						</Button>
					</span>
				</li>
			{/each}
		</ul>
		{#if splits.length > 0}
			<p class="muted note">
				<strong>Apply</strong> a split to restate the trades from before it in the new share count
				— the dollars paid don't change. Dismiss it if you already entered them that way.
			</p>
		{/if}
		{#if dividends.length > 0}
			<p class="muted note">
				<strong>Confirm</strong> a dividend once it shows in the account: it's added as income
				under <strong>Dividends</strong> on the day it was paid (today when that isn't published).
				Edit changes the amount or day, or records it as reinvested. Dismiss one you've already
				logged.
			</p>
		{/if}
	</div>
{/if}

<Modal bind:open={editOpen} title={editing ? `${editing.symbol} dividend` : 'Dividend'}>
	{#if editing}
		<form onsubmit={save}>
			<div class="form-row">
				<Field label="Amount" id="dividendAmount" hint="What the statement shows">
					<input
						id="dividendAmount"
						type="number"
						inputmode="decimal"
						step="0.01"
						min="0.01"
						bind:value={amount}
						disabled={busy !== null}
						required
					/>
				</Field>
				<Field label="Paid on" id="dividendDate" hint="On or after {formatDate(editing.ex_date)}">
					<input
						id="dividendDate"
						type="date"
						min={editing.ex_date}
						max={today()}
						bind:value={date}
						disabled={busy !== null}
						required
					/>
				</Field>
			</div>
			<label class="check">
				<input type="checkbox" bind:checked={reinvested} disabled={busy !== null} />
				Reinvested into more {editing.symbol}
			</label>
			{#if reinvested}
				<Field
					label="Shares bought"
					id="dividendShares"
					hint="From the statement — this is only a guess at today's price. It counts as dividend income and buys the shares, so the cash doesn't change."
				>
					<input
						id="dividendShares"
						type="number"
						inputmode="decimal"
						step="any"
						min="0"
						bind:value={shares}
						disabled={busy !== null}
						required
					/>
				</Field>
			{/if}
			{#if formError}
				<p class="error-text">{formError}</p>
			{/if}
			<div class="form-actions">
				<Button variant="secondary" onclick={() => (editOpen = false)}>Cancel</Button>
				<Button type="submit" disabled={busy !== null}>
					{busy !== null ? 'Saving…' : 'Confirm dividend'}
				</Button>
			</div>
		</form>
	{/if}
</Modal>

<style>
	h2 {
		margin: 0 0 0.75rem;
		font-size: 1.125rem;
	}

	.dividends {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.dividends li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.625rem 0;
		border-bottom: 1px solid var(--divider);
		flex-wrap: wrap;
	}

	.what {
		display: flex;
		flex-direction: column;
		flex: 1 1 12rem;
		min-width: 0;
	}

	.symbol {
		font-weight: 600;
	}

	.sub {
		font-size: 0.8125rem;
	}

	.amount {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: var(--pos);
	}

	.actions {
		display: flex;
		gap: 0.25rem;
		margin-left: auto;
	}

	.note {
		font-size: 0.8125rem;
		margin: 0.75rem 0 0;
	}

	.check {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.25rem 0 0.75rem;
		font-size: 0.9375rem;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}
</style>
