<script lang="ts">
	import { onMount } from 'svelte';
	import {
		achieveGoal,
		contributeToGoal,
		createGoal,
		deleteGoal,
		getAccounts,
		getBudgets,
		getCategories,
		getGoals,
		reopenGoal,
		setBudget,
		stopBudget,
		updateGoal
	} from '$lib/api';
	import type { Account, Budget, Category, Goal, GoalInput } from '$lib/types';
	import {
		formatCurrency,
		formatDate,
		formatMonth,
		monthStart,
		shiftMonth,
		today
	} from '$lib/format';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';
	import BudgetBar from '$lib/components/BudgetBar.svelte';
	import { budgetPace } from '$lib/budget';

	let month = $state(monthStart());
	let budgets: Budget[] = $state([]);
	let unbudgeted = $state(0);
	let budgetsLoading = $state(false);
	let goals: Goal[] = $state([]);
	let categories: Category[] = $state([]);
	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	// Budget form
	let budgetModalOpen = $state(false);
	let budgetSaving = $state(false);
	let budgetError = $state<string | null>(null);
	let budgetCategoryId = $state(0);
	let budgetAmount = $state('');

	// Goal form
	let goalModalOpen = $state(false);
	let editingGoal = $state<Goal | null>(null);
	let goalSaving = $state(false);
	let goalDeleting = $state(false);
	let goalError = $state<string | null>(null);
	let goalName = $state('');
	let goalTarget = $state('');
	let goalDate = $state('');
	let goalAccountId = $state(0);
	let goalNotes = $state('');

	// Contribution form
	let contribModalOpen = $state(false);
	let contribGoal = $state<Goal | null>(null);
	let contribSaving = $state(false);
	let contribError = $state<string | null>(null);
	let contribAmount = $state('');
	let contribDate = $state(today());

	onMount(load);

	async function load() {
		loading = true;
		loadError = null;
		try {
			[, goals, categories, accounts] = await Promise.all([
				loadBudgets(),
				getGoals(),
				getCategories(),
				getAccounts()
			]);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load budgets and goals';
		} finally {
			loading = false;
		}
	}

	// Budgets reload on their own when the month changes, without refetching the
	// goals, categories, and accounts that don't depend on it.
	async function loadBudgets() {
		const requested = month;
		budgetsLoading = true;
		try {
			const data = await getBudgets(requested);
			if (requested !== month) return; // a later month change won
			budgets = data.budgets;
			unbudgeted = data.unbudgeted_spent;
		} finally {
			if (requested === month) budgetsLoading = false;
		}
	}

	async function goToMonth(next: string) {
		month = next;
		loadError = null;
		try {
			await loadBudgets();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load budgets';
		}
	}

	let isCurrentMonth = $derived(month === monthStart());
	let expenseCategories = $derived(
		categories.filter((c) => c.kind === 'expense' && !c.archived)
	);
	let budgetedIds = $derived(new Set(budgets.map((b) => b.category_id)));
	let unbudgetedCategories = $derived(expenseCategories.filter((c) => !budgetedIds.has(c.id)));
	let totalBudget = $derived(budgets.reduce((sum, b) => sum + b.amount, 0));
	let totalSpent = $derived(budgets.reduce((sum, b) => sum + b.spent, 0));

	function openBudget() {
		budgetCategoryId = unbudgetedCategories[0]?.id ?? expenseCategories[0]?.id ?? 0;
		budgetAmount = '';
		budgetError = null;
		budgetModalOpen = true;
	}

	function editBudget(budget: Budget) {
		budgetCategoryId = budget.category_id;
		budgetAmount = String(budget.amount);
		budgetError = null;
		budgetModalOpen = true;
	}

	async function saveBudget(event: Event) {
		event.preventDefault();
		if (!budgetCategoryId || !budgetAmount) return;

		budgetSaving = true;
		budgetError = null;
		try {
			await setBudget(budgetCategoryId, Math.abs(parseFloat(budgetAmount) || 0), month);
			budgetModalOpen = false;
			await loadBudgets();
		} catch (e) {
			budgetError = e instanceof Error ? e.message : 'Could not save the budget';
		} finally {
			budgetSaving = false;
		}
	}

	async function endBudget(budget: Budget) {
		try {
			await stopBudget(budget.id, month);
			await loadBudgets();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not stop the budget';
		}
	}

	function openGoalCreate() {
		editingGoal = null;
		goalName = '';
		goalTarget = '';
		goalDate = '';
		goalAccountId = 0;
		goalNotes = '';
		goalError = null;
		goalModalOpen = true;
	}

	function openGoalEdit(goal: Goal) {
		editingGoal = goal;
		goalName = goal.name;
		goalTarget = String(goal.target_amount);
		goalDate = goal.target_date ?? '';
		goalAccountId = goal.account_id ?? 0;
		goalNotes = goal.notes ?? '';
		goalError = null;
		goalModalOpen = true;
	}

	function buildGoal(): GoalInput {
		return {
			name: goalName.trim(),
			target_amount: Math.abs(parseFloat(goalTarget) || 0),
			target_date: goalDate || null,
			account_id: goalAccountId || null,
			notes: goalNotes.trim() || null
		};
	}

	async function saveGoal(event: Event) {
		event.preventDefault();
		if (!goalName.trim() || !goalTarget) return;

		goalSaving = true;
		goalError = null;
		try {
			if (editingGoal) {
				await updateGoal(editingGoal.id, buildGoal());
			} else {
				await createGoal(buildGoal());
			}
			goalModalOpen = false;
			await load();
		} catch (e) {
			goalError = e instanceof Error ? e.message : 'Could not save the goal';
		} finally {
			goalSaving = false;
		}
	}

	async function removeGoal() {
		if (!editingGoal) return;
		goalDeleting = true;
		goalError = null;
		try {
			await deleteGoal(editingGoal.id);
			goalModalOpen = false;
			await load();
		} catch (e) {
			goalError = e instanceof Error ? e.message : 'Could not delete the goal';
		} finally {
			goalDeleting = false;
		}
	}

	function openContribute(goal: Goal) {
		contribGoal = goal;
		contribAmount = '';
		contribDate = today();
		contribError = null;
		contribModalOpen = true;
	}

	async function saveContribution(event: Event) {
		event.preventDefault();
		if (!contribGoal || !contribAmount) return;

		contribSaving = true;
		contribError = null;
		try {
			await contributeToGoal(
				contribGoal.id,
				Math.abs(parseFloat(contribAmount) || 0),
				contribDate
			);
			contribModalOpen = false;
			await load();
		} catch (e) {
			contribError = e instanceof Error ? e.message : 'Could not record the contribution';
		} finally {
			contribSaving = false;
		}
	}

	async function toggleAchieved(goal: Goal) {
		try {
			await (goal.achieved ? reopenGoal(goal.id) : achieveGoal(goal.id));
			await load();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not update the goal';
		}
	}

	const pct = (a: number, b: number) => (b <= 0 ? 0 : Math.min(100, Math.max(0, (a / b) * 100)));
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Budgets &amp; Goals</h1>
		</div>
	</div>

	{#if loadError}
		<p class="error-text">{loadError}</p>
	{/if}

	{#if loading}
		<p class="muted">Loading…</p>
	{:else}
		<section class="card">
			<div class="section-head">
				<h2>Monthly Budgets</h2>
				<Button size="sm" onclick={openBudget} disabled={expenseCategories.length === 0}>
					Set a Budget
				</Button>
			</div>

			<div class="month-nav">
				<Button variant="ghost" size="sm" onclick={() => goToMonth(shiftMonth(month, -1))}>
					‹ {formatMonth(shiftMonth(month, -1))}
				</Button>
				<strong class="month-label" class:loading={budgetsLoading}>{formatMonth(month)}</strong>
				<Button variant="ghost" size="sm" onclick={() => goToMonth(shiftMonth(month, 1))}>
					{formatMonth(shiftMonth(month, 1))} ›
				</Button>
				{#if !isCurrentMonth}
					<Button variant="secondary" size="sm" onclick={() => goToMonth(monthStart())}>
						This month
					</Button>
				{/if}
			</div>

			{#if budgets.length === 0}
				<p class="muted small">
					{#if isCurrentMonth}
						No budgets set. Pick a category and a monthly limit to track spending against it.
					{:else}
						No budgets in {formatMonth(month)}.
					{/if}
					{#if unbudgeted > 0}
						{formatCurrency(unbudgeted)} spent in {formatMonth(month)}.
					{/if}
				</p>
			{:else}
				<div class="summary">
					<span>
						<strong class:neg={totalSpent > totalBudget}>{formatCurrency(totalSpent)}</strong>
						spent of {formatCurrency(totalBudget)} budgeted
						{#if unbudgeted > 0}
							· {formatCurrency(unbudgeted)} unbudgeted
						{/if}
					</span>
				</div>

				<div class="list">
					{#each budgets as budget (budget.id)}
						{@const pace = budgetPace(budget.spent, budget.amount, month)}
						{@const remaining = budget.amount - budget.spent}
						<div class="item">
							<div class="item-head">
								<span class="item-name">{budget.category_name}</span>
								<span class="item-actions">
									<Button variant="ghost" size="sm" onclick={() => editBudget(budget)}>Edit</Button>
									<Button variant="ghost" size="sm" onclick={() => endBudget(budget)}>
										Stop
									</Button>
								</span>
							</div>
							<BudgetBar
								spent={budget.spent}
								amount={budget.amount}
								color={budget.category_color}
								{pace}
							/>
							<div class="item-foot muted">
								{formatCurrency(budget.spent)} of {formatCurrency(budget.amount)}
								{#if pace.status === 'over'}
									· <span class="neg">{formatCurrency(-remaining)} over</span>
								{:else}
									· {formatCurrency(remaining)} left
								{/if}
								{#if pace.status === 'ahead' && pace.projected !== null}
									·
									<span class="warn-text">
										ahead of pace, on track for {formatCurrency(pace.projected)}
									</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
				<p class="muted hint">
					Stopping a budget ends it from {formatMonth(month)} on. Earlier months keep it.
				</p>
			{/if}
		</section>

		<section class="card">
			<div class="section-head">
				<h2>Savings Goals</h2>
				<Button size="sm" onclick={openGoalCreate}>Add a Goal</Button>
			</div>

			{#if goals.length === 0}
				<p class="muted small">
					No goals yet. Link one to an account to track it automatically, or log contributions by
					hand.
				</p>
			{:else}
				<div class="list">
					{#each goals as goal (goal.id)}
						<div class="item" class:achieved={goal.achieved}>
							<div class="item-head">
								<span class="item-name">
									{goal.name}
									{#if goal.achieved}<span class="chip">Reached</span>{/if}
								</span>
								<span class="item-actions">
									{#if !goal.account_id}
										<Button variant="ghost" size="sm" onclick={() => openContribute(goal)}>
											Add
										</Button>
									{/if}
									<Button variant="ghost" size="sm" onclick={() => toggleAchieved(goal)}>
										{goal.achieved ? 'Reopen' : 'Mark reached'}
									</Button>
									<Button variant="ghost" size="sm" onclick={() => openGoalEdit(goal)}>Edit</Button>
								</span>
							</div>
							<div class="bar">
								<div
									class="bar-fill"
									style="width: {pct(goal.saved, goal.target_amount)}%; background: var(--info)"
								></div>
							</div>
							<div class="item-foot muted">
								{formatCurrency(goal.saved)} of {formatCurrency(goal.target_amount)}
								{#if goal.account_name}· tracking {goal.account_name}{/if}
								{#if goal.target_date}· by {formatDate(goal.target_date)}{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/if}
</div>

<Modal bind:open={budgetModalOpen} title="Set a Budget for {formatMonth(month)}">
	<form onsubmit={saveBudget}>
		<Field label="Category" id="budgetCategory">
			<select id="budgetCategory" bind:value={budgetCategoryId} disabled={budgetSaving}>
				{#each expenseCategories as category (category.id)}
					<option value={category.id}>{category.name}</option>
				{/each}
			</select>
		</Field>

		<Field
			label="Monthly limit"
			id="budgetAmount"
			hint="Applies from {formatMonth(month)} until the next change"
		>
			<input
				id="budgetAmount"
				type="number"
				step="0.01"
				min="0.01"
				placeholder="0.00"
				bind:value={budgetAmount}
				disabled={budgetSaving}
				required
			/>
		</Field>

		{#if budgetError}
			<p class="error-text">{budgetError}</p>
		{/if}

		<div class="form-actions">
			<Button variant="secondary" onclick={() => (budgetModalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={budgetSaving || !budgetAmount}>
				{budgetSaving ? 'Saving…' : 'Save Budget'}
			</Button>
		</div>
	</form>
</Modal>

<Modal bind:open={goalModalOpen} title={editingGoal ? 'Edit Goal' : 'Add Goal'}>
	<form onsubmit={saveGoal}>
		<Field label="Goal name" id="goalName">
			<input
				id="goalName"
				bind:value={goalName}
				placeholder="Emergency fund"
				disabled={goalSaving}
				required
			/>
		</Field>

		<div class="form-row">
			<Field label="Target amount" id="goalTarget">
				<input
					id="goalTarget"
					type="number"
					step="0.01"
					min="0.01"
					placeholder="0.00"
					bind:value={goalTarget}
					disabled={goalSaving}
					required
				/>
			</Field>
			<Field label="Target date" id="goalDate" hint="Optional">
				<input id="goalDate" type="date" bind:value={goalDate} disabled={goalSaving} />
			</Field>
		</div>

		<Field
			label="Track an account"
			id="goalAccount"
			hint="Progress follows that account's balance. Leave unset to log contributions by hand."
		>
			<select id="goalAccount" bind:value={goalAccountId} disabled={goalSaving}>
				<option value={0}>Track manually</option>
				{#each accounts as account (account.id)}
					<option value={account.id}>{account.name}</option>
				{/each}
			</select>
		</Field>

		<Field label="Notes" id="goalNotes">
			<textarea id="goalNotes" bind:value={goalNotes} disabled={goalSaving}></textarea>
		</Field>

		{#if goalError}
			<p class="error-text">{goalError}</p>
		{/if}

		<div class="form-actions">
			{#if editingGoal}
				<Button variant="danger" onclick={removeGoal} disabled={goalSaving || goalDeleting}>
					{goalDeleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (goalModalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={goalSaving || goalDeleting || !goalName.trim() || !goalTarget}>
				{goalSaving ? 'Saving…' : editingGoal ? 'Save Changes' : 'Add Goal'}
			</Button>
		</div>
	</form>
</Modal>

<Modal bind:open={contribModalOpen} title="Add to {contribGoal?.name ?? 'Goal'}">
	<form onsubmit={saveContribution}>
		<div class="form-row">
			<Field label="Amount" id="contribAmount">
				<input
					id="contribAmount"
					type="number"
					step="0.01"
					min="0.01"
					placeholder="0.00"
					bind:value={contribAmount}
					disabled={contribSaving}
					required
				/>
			</Field>
			<Field label="Date" id="contribDate">
				<input id="contribDate" type="date" bind:value={contribDate} disabled={contribSaving} />
			</Field>
		</div>

		{#if contribError}
			<p class="error-text">{contribError}</p>
		{/if}

		<div class="form-actions">
			<Button variant="secondary" onclick={() => (contribModalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={contribSaving || !contribAmount}>
				{contribSaving ? 'Saving…' : 'Add'}
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
	}

	.section-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
		gap: 1rem;
	}

	.month-nav {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.month-label {
		min-width: 9rem;
		text-align: center;
		font-size: 0.9375rem;
	}

	.month-label.loading {
		opacity: 0.5;
	}

	.summary {
		font-size: 0.875rem;
		color: var(--muted);
		padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--divider);
		margin-bottom: 0.5rem;
	}

	.summary strong {
		color: var(--ink);
	}

	.list {
		display: flex;
		flex-direction: column;
	}

	.item {
		padding: 0.875rem 0;
		border-bottom: 1px solid #f0f0f0;
	}

	.item:last-child {
		border-bottom: none;
	}

	.item.achieved {
		opacity: 0.6;
	}

	.item-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		margin-bottom: 0.5rem;
	}

	.item-name {
		font-weight: 600;
		font-size: 0.9375rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.item-actions {
		display: flex;
		gap: 0.25rem;
	}

	.item-foot {
		font-size: 0.8125rem;
		margin-top: 0.375rem;
	}

	.chip {
		font-size: 0.7rem;
		font-weight: 600;
		padding: 0.1rem 0.45rem;
		border-radius: 4px;
		background: #dcfce7;
		color: #166534;
	}

	.bar {
		height: 8px;
		background: var(--divider);
		border-radius: 999px;
		overflow: hidden;
	}

	.bar-fill {
		height: 100%;
		border-radius: 999px;
		transition: width 0.3s;
	}

	.hint {
		font-size: 0.75rem;
		margin-top: 0.5rem;
	}

	.small {
		font-size: 0.875rem;
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
		justify-content: flex-end;
	}

	.spacer {
		flex: 1;
	}
</style>
