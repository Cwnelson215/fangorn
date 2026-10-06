<script lang="ts">
	// Dividends the app found on held stocks and funds, waiting to be confirmed.
	// The price service says one is owed (shares held into the ex-date × the
	// amount per share) but not the day it lands or what a reinvestment bought,
	// so nothing posts by itself. Confirm is the one tap — the estimate, as cash,
	// today; Edit is for a different amount or date, or a reinvested dividend.
	// The card renders nothing when there are none.
	import { onMount } from 'svelte';
	import { confirmDividend, dismissDividend, getDividends } from '$lib/api';
	import type { Dividend } from '$lib/types';
	import { formatCurrency, formatDate, formatPrice, formatShares, today } from '$lib/format';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	// Without accountId it lists every account's and names the account on each.
	let { accountId, onchange }: { accountId?: number; onchange?: () => void } = $props();

	let dividends = $state<Dividend[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<number | null>(null);

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
			dividends = await getDividends(accountId);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load dividends';
		}
	}

	// A mutual fund's dividend is almost always reinvested, and that needs the
	// share count from the statement, so its Confirm opens the form instead.
	function reinvests(d: Dividend): boolean {
		return d.quote_type === 'MUTUALFUND';
	}

	async function act(d: Dividend, what: () => Promise<void>, failed: string): Promise<boolean> {
		busy = d.id;
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
		act(d, () => confirmDividend(d.id), 'Could not confirm the dividend');
	}

	function dismiss(d: Dividend) {
		act(d, () => dismissDividend(d.id), 'Could not dismiss the dividend');
	}

	function edit(d: Dividend) {
		editing = d;
		amount = d.amount;
		date = today();
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
			d,
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
</script>

{#if dividends.length > 0 || error}
	<div class="card">
		<h2>Dividends to confirm</h2>
		{#if error}
			<p class="error-text">{error}</p>
		{/if}
		<ul class="dividends">
			{#each dividends as d (d.id)}
				<li>
					<span class="what">
						<span class="symbol">{d.symbol} dividend</span>
						<span class="muted sub">
							{#if accountId == null}{d.account_name} · {/if}{formatShares(d.shares)} sh × {formatPrice(
								d.per_share
							)} · ex-date {formatDate(d.ex_date)}
						</span>
					</span>
					<span class="amount num">≈ {formatCurrency(d.amount)}</span>
					<span class="actions">
						<Button size="sm" disabled={busy === d.id} onclick={() => confirm(d)}>
							{reinvests(d) ? 'Confirm…' : 'Confirm'}
						</Button>
						{#if !reinvests(d)}
							<Button variant="ghost" size="sm" disabled={busy === d.id} onclick={() => edit(d)}>
								Edit
							</Button>
						{/if}
						<Button variant="ghost" size="sm" disabled={busy === d.id} onclick={() => dismiss(d)}>
							Dismiss
						</Button>
					</span>
				</li>
			{/each}
		</ul>
		{#if dividends.length > 0}
			<p class="muted note">
				Confirm once it shows in the account — it's added today as income under
				<strong>Dividends</strong>. Edit changes the amount or day, or records it as reinvested.
				Dismiss one you've already logged.
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
					hint="From the statement — this is only a guess at today's price. No cash is added."
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
