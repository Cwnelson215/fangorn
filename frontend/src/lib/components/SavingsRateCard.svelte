<script lang="ts">
	// A high-yield savings account's interest: the current rate, what this month
	// should earn, and the rate history. Interest itself posts on its own once a
	// month ends (the scheduler); this is where the rate it uses is kept.
	import { onMount } from 'svelte';
	import { addSavingsRate, deleteSavingsRate, getSavingsOutlook } from '$lib/api';
	import type { SavingsOutlook } from '$lib/types';
	import { formatCurrency, formatDate, today } from '$lib/format';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	let { accountId }: { accountId: number } = $props();

	let outlook = $state<SavingsOutlook | null>(null);
	let error = $state<string | null>(null);

	let modalOpen = $state(false);
	let apy = $state<string | number>('');
	let effectiveFrom = $state(today());
	let saving = $state(false);
	let formError = $state<string | null>(null);

	onMount(load);

	async function load() {
		try {
			outlook = await getSavingsOutlook(accountId);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the interest rate';
		}
	}

	// Newest first; the current rate is the newest that has already started.
	let current = $derived(outlook?.rates.find((r) => r.effective_from <= today()) ?? null);

	function openChange() {
		apy = current?.apy ?? '';
		effectiveFrom = today();
		formError = null;
		modalOpen = true;
	}

	async function save(event: Event) {
		event.preventDefault();
		saving = true;
		formError = null;
		try {
			await addSavingsRate(accountId, { apy: Number(apy), effective_from: effectiveFrom });
			modalOpen = false;
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
</script>

<div class="card">
	<div class="head">
		<h2>Interest</h2>
		<Button size="sm" onclick={openChange}>Change rate</Button>
	</div>

	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	{#if outlook}
		<div class="summary">
			<div>
				<span class="label">Rate</span>
				<span class="value">{current ? `${current.apy}% APY` : 'Not started yet'}</span>
			</div>
			<div>
				<span class="label">This month</span>
				<span class="value pos">≈ {formatCurrency(outlook.projected_amount)}</span>
				<span class="muted note">posts {formatDate(outlook.projected_date)} if the balance holds</span>
			</div>
		</div>

		<p class="muted note">
			Once a month ends, its interest is added as an <strong>Interest</strong> income entry: the
			month-end balance × the month's rate. A rate change mid-month counts from its date.
		</p>

		<h3>Rate history</h3>
		<ul class="rates">
			{#each outlook.rates as r (r.id)}
				<li>
					<span class="apy">{r.apy}%</span>
					<span class="muted">from {formatDate(r.effective_from)}</span>
					{#if r.effective_from > today()}<span class="badge">upcoming</span>{/if}
					{#if outlook.rates.length > 1}
						<Button variant="ghost" size="sm" onclick={() => remove(r.id)}>Remove</Button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>

<Modal bind:open={modalOpen} title="Change interest rate">
	<form onsubmit={save}>
		<div class="form-row">
			<Field label="New APY (%)" id="newApy">
				<input
					id="newApy"
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
			<Field label="Effective from" id="rateFrom" hint="The day the bank's new rate started">
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
			<Button variant="secondary" onclick={() => (modalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save rate'}</Button>
		</div>
	</form>
</Modal>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.head h2 {
		margin: 0;
		font-size: 1.125rem;
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
