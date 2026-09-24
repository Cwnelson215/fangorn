<script lang="ts">
	import { onMount } from 'svelte';
	import {
		createRule,
		deleteRule,
		getAccounts,
		getCategories,
		getRules,
		pauseRule,
		postRuleNow,
		resumeRule,
		skipRule,
		updateRule
	} from '$lib/api';
	import type { Account, Category, Frequency, Kind, RecurringRule, RuleInput } from '$lib/types';
	import { FREQUENCY_LABELS } from '$lib/types';
	import { formatCurrency, formatDate, relativeDays, today } from '$lib/format';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

	let rules: RecurringRule[] = $state([]);
	let accounts: Account[] = $state([]);
	let categories: Category[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let busyId = $state<number | null>(null);

	let modalOpen = $state(false);
	let editing = $state<RecurringRule | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let name = $state('');
	let vendor = $state('');
	let kind = $state<Kind>('expense');
	let accountId = $state(0);
	let toAccountId = $state(0);
	let categoryId = $state(0);
	let amount = $state('');
	let frequency = $state<Frequency>('monthly');
	let intervalCount = $state(1);
	let dayOfMonth = $state<number | ''>('');
	let secondDayOfMonth = $state<number | ''>('');
	let dayOfWeek = $state<number | ''>('');
	let startDate = $state(today());
	let endDate = $state('');
	let autoPost = $state(true);
	let notes = $state('');

	const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

	onMount(load);

	async function load() {
		loading = true;
		loadError = null;
		try {
			[rules, accounts, categories] = await Promise.all([
				getRules(),
				getAccounts(),
				getCategories()
			]);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load recurring items';
		} finally {
			loading = false;
		}
	}

	let availableCategories = $derived(
		kind === 'transfer' ? [] : categories.filter((c) => c.kind === kind)
	);

	// Which schedule inputs are relevant depends on the frequency chosen.
	let needsDayOfMonth = $derived(
		frequency === 'monthly' || frequency === 'quarterly' || frequency === 'yearly'
	);
	let needsTwoDays = $derived(frequency === 'semimonthly');
	let needsWeekday = $derived(frequency === 'weekly' || frequency === 'biweekly');

	function openCreate() {
		editing = null;
		name = '';
		vendor = '';
		kind = 'expense';
		accountId = accounts[0]?.id ?? 0;
		toAccountId = accounts[1]?.id ?? 0;
		categoryId = 0;
		amount = '';
		frequency = 'monthly';
		intervalCount = 1;
		dayOfMonth = '';
		secondDayOfMonth = '';
		dayOfWeek = '';
		startDate = today();
		endDate = '';
		autoPost = true;
		notes = '';
		formError = null;
		modalOpen = true;
	}

	function openEdit(rule: RecurringRule) {
		editing = rule;
		name = rule.name;
		vendor = rule.vendor ?? '';
		kind = rule.kind;
		accountId = rule.account_id;
		toAccountId = rule.to_account_id ?? accounts.find((a) => a.id !== rule.account_id)?.id ?? 0;
		categoryId = rule.category_id ?? 0;
		amount = String(rule.amount);
		frequency = rule.frequency;
		intervalCount = rule.interval_count;
		dayOfMonth = rule.day_of_month ?? '';
		secondDayOfMonth = rule.second_day_of_month ?? '';
		dayOfWeek = rule.day_of_week ?? '';
		startDate = rule.start_date;
		endDate = rule.end_date ?? '';
		autoPost = rule.auto_post;
		notes = rule.notes ?? '';
		formError = null;
		modalOpen = true;
	}

	function buildInput(): RuleInput {
		const isTransfer = kind === 'transfer';
		return {
			name: name.trim(),
			vendor: vendor.trim() || null,
			kind,
			account_id: accountId,
			to_account_id: isTransfer ? toAccountId : null,
			category_id: isTransfer ? null : categoryId || null,
			amount: Math.abs(parseFloat(amount) || 0),
			frequency,
			interval_count: Math.max(1, Number(intervalCount) || 1),
			day_of_month: needsDayOfMonth || needsTwoDays ? Number(dayOfMonth) || null : null,
			second_day_of_month: needsTwoDays ? Number(secondDayOfMonth) || null : null,
			day_of_week: needsWeekday && dayOfWeek !== '' ? Number(dayOfWeek) : null,
			month_of_year: null,
			start_date: startDate,
			end_date: endDate || null,
			auto_post: autoPost,
			reminder_lead_days: null,
			notes: notes.trim() || null
		};
	}

	let sameAccount = $derived(kind === 'transfer' && accountId !== 0 && accountId === toAccountId);

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (!name.trim() || !amount || !accountId) return;
		if (sameAccount) {
			formError = 'Pick two different accounts';
			return;
		}

		saving = true;
		formError = null;
		try {
			if (editing) {
				await updateRule(editing.id, buildInput());
			} else {
				await createRule(buildInput());
			}
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save this recurring item';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!editing) return;
		deleting = true;
		formError = null;
		try {
			await deleteRule(editing.id);
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete this recurring item';
		} finally {
			deleting = false;
		}
	}

	async function act(rule: RecurringRule, action: (id: number) => Promise<unknown>) {
		busyId = rule.id;
		loadError = null;
		try {
			await action(rule.id);
			await load();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'That action did not work';
		} finally {
			busyId = null;
		}
	}

	function scheduleSummary(rule: RecurringRule): string {
		const base = FREQUENCY_LABELS[rule.frequency] ?? rule.frequency;
		if (rule.interval_count > 1) return `Every ${rule.interval_count} × ${base.toLowerCase()}`;
		return base;
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Recurring</h1>
			<p class="muted">
				Subscriptions and scheduled transfers. These post automatically on their due date.
			</p>
		</div>
		<Button onclick={openCreate} disabled={accounts.length === 0}>Add Recurring</Button>
	</div>

	{#if loadError}
		<p class="error-text">{loadError}</p>
	{/if}

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if accounts.length === 0}
		<div class="card empty">
			<h2>Add an account first</h2>
			<a class="cta" href="/accounts">Go to accounts</a>
		</div>
	{:else if rules.length === 0}
		<div class="card empty">
			<h2>Nothing recurring yet</h2>
			<p class="muted">
				Add your subscriptions and any transfers you make on a schedule, and they'll be logged for
				you.
			</p>
		</div>
	{:else}
		<div class="rules">
			{#each rules as rule (rule.id)}
				<div class="card rule" class:paused={rule.paused}>
					<div class="rule-main">
						<div class="rule-title">
							<span class="rule-name">{rule.name}</span>
							<span class="chip {rule.kind}">
								{rule.kind === 'transfer'
									? 'Transfer'
									: rule.kind === 'income'
										? 'Money in'
										: 'Money out'}
							</span>
							{#if rule.paused}<span class="chip paused-chip">Paused</span>{/if}
							{#if !rule.auto_post}<span class="chip manual">Manual</span>{/if}
						</div>
						<div class="muted rule-meta">
							{scheduleSummary(rule)} ·
							{#if rule.kind === 'transfer'}
								{rule.account_name} → {rule.to_account_name}
							{:else}
								{rule.account_name}{#if rule.category_name}&nbsp;· {rule.category_name}{/if}
							{/if}
						</div>
						<div class="rule-next">
							{#if rule.paused}
								<span class="muted">Paused — nothing will post</span>
							{:else if rule.next_due_date}
								Next: <strong>{formatDate(rule.next_due_date)}</strong>
								<span class="muted">({relativeDays(rule.next_due_date)})</span>
							{:else}
								<span class="muted">No further occurrences</span>
							{/if}
						</div>
					</div>

					<div class="rule-side">
						<span class="rule-amount">{formatCurrency(rule.amount)}</span>
						<div class="rule-actions">
							<Button variant="ghost" size="sm" onclick={() => openEdit(rule)}>Edit</Button>
							{#if rule.paused}
								<Button
									variant="ghost"
									size="sm"
									disabled={busyId === rule.id}
									onclick={() => act(rule, resumeRule)}
								>
									Resume
								</Button>
							{:else}
								<Button
									variant="ghost"
									size="sm"
									disabled={busyId === rule.id}
									onclick={() => act(rule, pauseRule)}
								>
									Pause
								</Button>
								{#if rule.next_due_date}
									<Button
										variant="ghost"
										size="sm"
										disabled={busyId === rule.id}
										onclick={() => act(rule, skipRule)}
									>
										Skip next
									</Button>
									<Button
										variant="ghost"
										size="sm"
										disabled={busyId === rule.id}
										onclick={() => act(rule, postRuleNow)}
									>
										Post now
									</Button>
								{/if}
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<Modal bind:open={modalOpen} title={editing ? 'Edit Recurring Item' : 'Add Recurring Item'}>
	<form onsubmit={handleSubmit}>
		<div class="toggle">
			{#each [{ value: 'expense', label: 'Charge' }, { value: 'income', label: 'Deposit' }, { value: 'transfer', label: 'Transfer' }] as option}
				<button
					type="button"
					class:active={kind === option.value}
					onclick={() => {
						kind = option.value as Kind;
						if (!availableCategories.some((c) => c.id === categoryId)) categoryId = 0;
					}}
				>
					{option.label}
				</button>
			{/each}
		</div>

		<Field label="Name" id="ruleName">
			<input
				id="ruleName"
				bind:value={name}
				placeholder={kind === 'transfer' ? 'Monthly savings transfer' : 'Netflix'}
				disabled={saving}
				required
			/>
		</Field>

		<div class="form-row">
			<Field label="Amount" id="ruleAmount">
				<input
					id="ruleAmount"
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
			<Field label={kind === 'transfer' ? 'From account' : 'Account'} id="ruleAccount">
				<select id="ruleAccount" bind:value={accountId} disabled={saving}>
					{#each accounts as account (account.id)}
						<option value={account.id}>{account.name}</option>
					{/each}
				</select>
			</Field>
		</div>

		{#if kind === 'transfer'}
			<Field label="To account" id="ruleToAccount">
				<select id="ruleToAccount" bind:value={toAccountId} disabled={saving}>
					{#each accounts as account (account.id)}
						<option value={account.id}>{account.name}</option>
					{/each}
				</select>
			</Field>
			{#if sameAccount}
				<p class="error-text">Source and destination must be different accounts.</p>
			{/if}
		{:else}
			<div class="form-row">
				<Field label="Category" id="ruleCategory">
					<select id="ruleCategory" bind:value={categoryId} disabled={saving}>
						<option value={0}>Uncategorized</option>
						{#each availableCategories as category (category.id)}
							<option value={category.id}>{category.name}</option>
						{/each}
					</select>
				</Field>
				<Field label="Vendor" id="ruleVendor">
					<input id="ruleVendor" bind:value={vendor} placeholder="Optional" disabled={saving} />
				</Field>
			</div>
		{/if}

		<div class="form-row">
			<Field label="Repeats" id="ruleFrequency">
				<select id="ruleFrequency" bind:value={frequency} disabled={saving}>
					{#each Object.entries(FREQUENCY_LABELS) as [value, label]}
						<option {value}>{label}</option>
					{/each}
				</select>
			</Field>
			<Field label="Every" id="ruleInterval" hint="1 = every time">
				<input
					id="ruleInterval"
					type="number"
					inputmode="numeric"
					min="1"
					max="12"
					bind:value={intervalCount}
					disabled={saving}
				/>
			</Field>
		</div>

		{#if needsWeekday}
			<Field label="Day of week" id="ruleWeekday" hint="Defaults to the start date's weekday">
				<select id="ruleWeekday" bind:value={dayOfWeek} disabled={saving}>
					<option value="">Same as start date</option>
					{#each WEEKDAYS as label, index}
						<option value={index}>{label}</option>
					{/each}
				</select>
			</Field>
		{:else if needsTwoDays}
			<div class="form-row">
				<Field label="First day" id="ruleDay1">
					<input
						id="ruleDay1"
						type="number"
						inputmode="numeric"
						min="1"
						max="31"
						placeholder="1"
						bind:value={dayOfMonth}
						disabled={saving}
					/>
				</Field>
				<Field label="Second day" id="ruleDay2">
					<input
						id="ruleDay2"
						type="number"
						inputmode="numeric"
						min="1"
						max="31"
						placeholder="15"
						bind:value={secondDayOfMonth}
						disabled={saving}
					/>
				</Field>
			</div>
		{:else if needsDayOfMonth}
			<Field
				label="Day of month"
				id="ruleDay"
				hint="Short months fall back to the last day — 31 becomes Feb 28"
			>
				<input
					id="ruleDay"
					type="number"
					inputmode="numeric"
					min="1"
					max="31"
					placeholder="Same as start date"
					bind:value={dayOfMonth}
					disabled={saving}
				/>
			</Field>
		{/if}

		<div class="form-row">
			<Field label="Starts" id="ruleStart" hint="Back-date this to fill in past occurrences">
				<input id="ruleStart" type="date" bind:value={startDate} disabled={saving} required />
			</Field>
			<Field label="Ends" id="ruleEnd" hint="Optional">
				<input id="ruleEnd" type="date" bind:value={endDate} disabled={saving} />
			</Field>
		</div>

		<label class="checkbox">
			<input type="checkbox" bind:checked={autoPost} disabled={saving} />
			<span>Post automatically when due</span>
		</label>

		<Field label="Notes" id="ruleNotes">
			<textarea id="ruleNotes" bind:value={notes} disabled={saving}></textarea>
		</Field>

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			{#if editing}
				<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
					{deleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (modalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !name.trim() || !amount || sameAccount}>
				{saving ? 'Saving…' : editing ? 'Save Changes' : 'Add It'}
			</Button>
		</div>
	</form>
</Modal>

<style>
	h1 {
		font-size: 1.5rem;
	}

	h2 {
		font-size: 1rem;
		margin-bottom: 0.5rem;
	}

	.rules {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.rule {
		display: flex;
		justify-content: space-between;
		gap: 1.5rem;
		flex-wrap: wrap;
		padding: 1.25rem;
	}

	.rule.paused {
		opacity: 0.65;
	}

	.rule-main {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0;
	}

	.rule-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.rule-name {
		font-weight: 600;
		font-size: 1rem;
	}

	.chip {
		font-size: 0.7rem;
		font-weight: 600;
		padding: 0.1rem 0.45rem;
		border-radius: 4px;
		background: var(--divider);
		color: var(--muted);
	}

	.chip.expense {
		background: #fef2f2;
		color: #991b1b;
	}

	.chip.income {
		background: #dcfce7;
		color: #166534;
	}

	.chip.transfer {
		background: #e3f2fd;
		color: #1565c0;
	}

	.chip.manual {
		background: #fef3c7;
		color: #92400e;
	}

	.rule-meta,
	.rule-next {
		font-size: 0.8125rem;
	}

	.rule-side {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.5rem;
		margin-left: auto;
	}

	.rule-amount {
		font-size: 1.25rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.rule-actions {
		display: flex;
		gap: 0.25rem;
		flex-wrap: wrap;
		justify-content: flex-end;
	}

	@media (max-width: 639px) {
		/* Amount pinned top-right beside the name; actions get a row of their own. */
		.rule {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			gap: 0.5rem 0.75rem;
			padding: 1rem;
		}

		.rule-side {
			display: contents;
		}

		.rule-amount {
			grid-row: 1;
			grid-column: 2;
			font-size: 1.0625rem;
		}

		.rule-actions {
			grid-column: 1 / -1;
			justify-content: flex-start;
			margin-left: -0.625rem;
			border-top: 1px solid var(--divider);
			padding-top: 0.375rem;
		}
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
		margin-top: 0.5rem;
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

	.checkbox {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.checkbox input {
		width: auto;
		accent-color: var(--accent);
	}

	.cta {
		display: inline-block;
		background: var(--accent);
		color: var(--ink);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
		margin-top: 1rem;
	}
</style>
