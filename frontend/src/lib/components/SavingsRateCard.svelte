<script lang="ts">
	// What an account's cash earns: the current rate, what this month should
	// earn, and the rate history. The scheduler posts it once a month ends; this
	// is where the rate it uses is kept.
	//
	// 'interest' is a high-yield savings account: its whole balance earns an APY
	// you enter, and it always has a rate. 'cash' is an investment or retirement
	// account: only its uninvested cash earns, at the yield of the money market
	// fund it sits in (SPAXX at Fidelity). Linking the fund lets the server look
	// that yield up every day; entering one by hand is the fallback.
	import { onMount } from 'svelte';
	import {
		addSavingsRate,
		deleteSavingsRate,
		getSavingsOutlook,
		setCashFund,
		unlinkCashFund
	} from '$lib/api';
	import type { SavingsOutlook } from '$lib/types';
	import { formatCurrency, formatDate, today } from '$lib/format';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	let { accountId, mode = 'interest' }: { accountId: number; mode?: 'interest' | 'cash' } = $props();

	let isCash = $derived(mode === 'cash');
	let unit = $derived(isCash ? 'yield' : 'APY');

	let outlook = $state<SavingsOutlook | null>(null);
	let error = $state<string | null>(null);

	// The hand-entered rate form.
	let rateOpen = $state(false);
	let apy = $state<string | number>('');
	let effectiveFrom = $state(today());
	let saving = $state(false);
	let formError = $state<string | null>(null);

	// The link-a-fund form.
	let fundOpen = $state(false);
	let fundSymbol = $state('SPAXX');
	let linking = $state(false);
	let fundError = $state<string | null>(null);

	onMount(load);

	async function load() {
		try {
			outlook = await getSavingsOutlook(accountId);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the interest rate';
		}
	}

	let linked = $derived(outlook?.cash_fund != null);
	// Newest first; the current rate is the newest that has already started.
	let current = $derived(outlook?.rates.find((r) => r.effective_from <= today()) ?? null);
	let hasRates = $derived((outlook?.rates.length ?? 0) > 0);

	function openRate() {
		apy = current?.apy ?? '';
		effectiveFrom = today();
		formError = null;
		rateOpen = true;
	}

	async function saveRate(event: Event) {
		event.preventDefault();
		saving = true;
		formError = null;
		try {
			await addSavingsRate(accountId, { apy: Number(apy), effective_from: effectiveFrom });
			rateOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the rate';
		} finally {
			saving = false;
		}
	}

	async function remove(rateId: number) {
		try {
			await deleteSavingsRate(accountId, rateId);
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not remove the rate';
		}
	}

	function openFund() {
		fundSymbol = 'SPAXX';
		fundError = null;
		fundOpen = true;
	}

	async function linkFund(event: Event) {
		event.preventDefault();
		linking = true;
		fundError = null;
		try {
			outlook = await setCashFund(accountId, fundSymbol.trim());
			fundOpen = false;
		} catch (e) {
			fundError = e instanceof Error ? e.message : 'Could not link the fund';
		} finally {
			linking = false;
		}
	}

	async function unlink() {
		try {
			outlook = await unlinkCashFund(accountId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not unlink the fund';
		}
	}
</script>

<div class="card">
	<div class="head">
		<h2>{isCash ? 'Cash yield' : 'Interest'}</h2>
		<div class="head-actions">
			{#if isCash && linked}
				<Button variant="ghost" size="sm" onclick={unlink}>Unlink</Button>
			{:else if isCash}
				<Button variant="ghost" size="sm" onclick={openRate}>
					{hasRates ? 'Change yield' : 'Enter by hand'}
				</Button>
				<Button size="sm" onclick={openFund}>Look up fund</Button>
			{:else}
				<Button size="sm" onclick={openRate}>Change rate</Button>
			{/if}
		</div>
	</div>

	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	{#if outlook && isCash && !linked && !hasRates}
		<p class="muted note">
			The account's uninvested cash usually sits in a money market fund (SPAXX at Fidelity) that
			pays a dividend each month. <strong>Look up fund</strong> links it: Fangorn checks the fund's
			published yield every day and adds the dividend when each month ends. Only the cash counts, not
			the holdings.
		</p>
	{:else if outlook}
		<div class="summary">
			<div>
				{#if linked}
					<span class="label">{outlook.cash_fund} yield</span>
					<span class="value">
						{outlook.fund_yield != null ? `${outlook.fund_yield}%` : 'Looking up…'}
					</span>
					{#if outlook.fund_yield_as_of}
						<span class="muted note">looked up {formatDate(outlook.fund_yield_as_of)}</span>
					{/if}
				{:else}
					<span class="label">{isCash ? 'Yield' : 'Rate'}</span>
					<span class="value">{current ? `${current.apy}% ${unit}` : 'Not started yet'}</span>
				{/if}
			</div>
			<div>
				<span class="label">This month</span>
				<span class="value pos">≈ {formatCurrency(outlook.projected_amount)}</span>
				<span class="muted note">
					posts {formatDate(outlook.projected_date)} if the {isCash ? 'cash' : 'balance'} holds
				</span>
			</div>
		</div>

		<p class="muted note">
			{#if linked}
				Checked against {outlook.cash_fund}'s published yield every day since
				{formatDate(outlook.cash_fund_since ?? '')}. When a month ends, an
				<strong>{outlook.cash_fund} dividend</strong> is added under <strong>Dividends</strong>: the
				month-end cash × the month's average yield ÷ 12. It lands close to the statement; edit it if
				it's off.
			{:else if isCash}
				Once a month ends, a <strong>Money market dividend</strong> is added under
				<strong>Dividends</strong>: the month-end cash × the month's yield. Money market yields move a
				little every day, so check it against the statement and edit it if it's off.
			{:else}
				Once a month ends, its interest is added as an <strong>Interest</strong> income entry: the
				month-end balance × the month's rate. A rate change mid-month counts from its date.
			{/if}
		</p>

		{#if hasRates}
			<h3>{linked ? 'Entered by hand, before linking' : 'Rate history'}</h3>
			<ul class="rates">
				{#each outlook.rates as r (r.id)}
					<li>
						<span class="apy">{r.apy}%</span>
						<span class="muted">from {formatDate(r.effective_from)}</span>
						{#if r.effective_from > today()}<span class="badge">upcoming</span>{/if}
						{#if !linked && (isCash || outlook.rates.length > 1)}
							<Button variant="ghost" size="sm" onclick={() => remove(r.id)}>Remove</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</div>

<Modal bind:open={rateOpen} title={isCash ? 'Cash yield' : 'Change interest rate'}>
	<form onsubmit={saveRate}>
		<div class="form-row">
			<Field label={isCash ? 'Yield (%)' : 'New APY (%)'} id="newApy">
				<input
					id="newApy"
					type="number"
					inputmode="decimal"
					step="0.001"
					min="0"
					max="99.999"
					placeholder={isCash ? '3.40' : '4.35'}
					bind:value={apy}
					disabled={saving}
					required
				/>
			</Field>
			<Field
				label="Effective from"
				id="rateFrom"
				hint={isCash ? 'Earlier dates add past months too' : "The day the bank's new rate started"}
			>
				<input id="rateFrom" type="date" bind:value={effectiveFrom} disabled={saving} required />
			</Field>
		</div>
		<p class="muted note">
			Months already posted keep the rate they earned at. The rest of this month counts at the new
			rate from its date.
		</p>
		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}
		<div class="form-actions">
			<Button variant="secondary" onclick={() => (rateOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save rate'}</Button>
		</div>
	</form>
</Modal>

<Modal bind:open={fundOpen} title="Look up the cash fund">
	<form onsubmit={linkFund}>
		<Field
			label="Money market fund"
			id="cashFund"
			hint="Where this account's uninvested cash sits — SPAXX is Fidelity's usual core position."
		>
			<input
				id="cashFund"
				bind:value={fundSymbol}
				autocapitalize="characters"
				autocomplete="off"
				disabled={linking}
				required
			/>
		</Field>
		<p class="muted note">
			The fund's yield counts from today. Anything entered by hand still covers the months before.
		</p>
		{#if fundError}
			<p class="error-text">{fundError}</p>
		{/if}
		<div class="form-actions">
			<Button variant="secondary" onclick={() => (fundOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={linking || !fundSymbol.trim()}>
				{linking ? 'Looking up…' : 'Link fund'}
			</Button>
		</div>
	</form>
</Modal>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.head h2 {
		margin: 0;
		font-size: 1.125rem;
	}

	.head-actions {
		display: flex;
		gap: 0.25rem;
		flex-wrap: wrap;
		justify-content: flex-end;
	}

	.summary {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		margin-bottom: 0.75rem;
	}

	.summary > div {
		display: flex;
		flex-direction: column;
	}

	.label {
		font-size: 0.8125rem;
		color: var(--muted);
		font-weight: 500;
	}

	.value {
		font-size: 1.25rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.value.pos {
		color: var(--pos);
	}

	.note {
		font-size: 0.8125rem;
		margin: 0.25rem 0 0;
	}

	h3 {
		font-size: 0.875rem;
		color: var(--muted);
		margin: 1.25rem 0 0.5rem;
	}

	.rates {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.rates li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--divider);
	}

	.rates li :global(button) {
		margin-left: auto;
	}

	.apy {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		min-width: 4.5rem;
	}

	.badge {
		font-size: 0.75rem;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		background: var(--bg);
		color: var(--muted);
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	@media (max-width: 639px) {
		.summary {
			grid-template-columns: 1fr;
			gap: 0.75rem;
		}
	}
</style>
