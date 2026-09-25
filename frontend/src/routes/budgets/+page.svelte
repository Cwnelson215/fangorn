<script lang="ts">
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	import { onMount } from 'svelte';
	import {
		achieveGoal,
		contributeToGoal,
		createGoal,
		createTransfer,
		deleteGoal,
		getAccounts,
		getBudgets,
		getCategories,
		getGoal,
		getGoals,
		getSettings,
		reopenGoal,
		setBudget,
		stopBudget,
		updateGoal,
		updateSettings
	} from '$lib/api';
	import type { Account, Budget, Category, Goal, GoalInput, SavingsLine } from '$lib/types';
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
	import { budgetPace, incomePace } from '$lib/budget';
	import type { CategoryKind } from '$lib/types';

	let month = $state(monthStart());
	let budgets: Budget[] = $state([]);
	let unbudgeted = $state(0);
	let incomeReceived = $state(0);
	let unplannedIncome = $state(0);
	let savings: SavingsLine[] = $state([]);
	let shortfall = $state(0);
	// Where income lands, and savings are moved from.
	let incomeAccountId = $state(0);
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
	let budgetKind = $state<CategoryKind>('expense');
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
	let goalMonthly = $state('');
	// "YYYY-MM": the month a long-term goal's monthly share applies from.
	let goalMonthlyFrom = $state('');
	// Set when the form is for a monthly goal: the first of its month.
	let goalMonth = $state<string | null>(null);

	// "Add money" to a goal: a transfer into its account, or a contribution
	// logged by hand when it has none.
	let contribModalOpen = $state(false);
	let contribGoal = $state<Pick<Goal, 'id' | 'name' | 'account_id'> | null>(null);
	let contribFromId = $state(0);
	let contribSaving = $state(false);
	let contribError = $state<string | null>(null);
	let contribAmount = $state('');
	let contribDate = $state(today());

	onMount(load);

	async function load() {
		loading = true;
		loadError = null;
		try {
			let settings;
			[, goals, categories, accounts, settings] = await Promise.all([
				loadBudgets(),
				getGoals(),
				getCategories(),
				getAccounts(),
				getSettings()
			]);
			incomeAccountId = settings.income_account_id ?? 0;
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
			incomeReceived = data.income_received;
			unplannedIncome = data.unplanned_income;
			savings = data.savings;
			shortfall = data.savings_shortfall;
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
	let incomeCategories = $derived(categories.filter((c) => c.kind === 'income' && !c.archived));
	let budgetedIds = $derived(new Set(budgets.map((b) => b.category_id)));

	// Spending limits and expected income share the budgets table, split by the
	// category's kind.
	let spendBudgets = $derived(budgets.filter((b) => b.kind === 'expense'));
	let incomeBudgets = $derived(budgets.filter((b) => b.kind === 'income'));
	let totalBudget = $derived(spendBudgets.reduce((sum, b) => sum + b.amount, 0));
	let totalSpent = $derived(spendBudgets.reduce((sum, b) => sum + b.spent, 0));
	let totalScheduled = $derived(spendBudgets.reduce((sum, b) => sum + b.scheduled, 0));
	let totalExpected = $derived(incomeBudgets.reduce((sum, b) => sum + b.amount, 0));
	let incomeScheduled = $derived(incomeBudgets.reduce((sum, b) => sum + b.scheduled, 0));
	let totalSavingsPlanned = $derived(savings.reduce((sum, l) => sum + l.monthly_amount, 0));
	let totalMoved = $derived(savings.reduce((sum, l) => sum + l.moved, 0));
	// What the plan leaves over: expected income less budgeted spending and the
	// month's savings.
	let planLeft = $derived(totalExpected - totalBudget - totalSavingsPlanned);
	let showPlan = $derived(incomeBudgets.length > 0 || savings.length > 0);

	async function chooseIncomeAccount(id: number) {
		const previous = incomeAccountId;
		incomeAccountId = id;
		try {
			const saved = await updateSettings({ income_account_id: id || null });
			incomeAccountId = saved.income_account_id ?? 0;
		} catch (e) {
			incomeAccountId = previous;
			loadError = e instanceof Error ? e.message : 'Could not save the income account';
		}
	}

	let modalCategories = $derived(budgetKind === 'income' ? incomeCategories : expenseCategories);

	function openBudget(kind: CategoryKind) {
		budgetKind = kind;
		const pool = kind === 'income' ? incomeCategories : expenseCategories;
		budgetCategoryId = pool.find((c) => !budgetedIds.has(c.id))?.id ?? pool[0]?.id ?? 0;
		budgetAmount = '';
		budgetError = null;
		budgetModalOpen = true;
	}

	function editBudget(budget: Budget) {
		budgetKind = budget.kind;
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

	function openGoalCreate(forMonth: string | null = null) {
		editingGoal = null;
		goalMonth = forMonth;
		goalMonthlyFrom = month.slice(0, 7);
		goalName = '';
		goalTarget = '';
		goalDate = '';
		goalAccountId = 0;
		goalNotes = '';
		goalMonthly = '';
		goalError = null;
		goalModalOpen = true;
	}

	function openGoalEdit(goal: Goal) {
		editingGoal = goal;
		goalMonth = goal.month;
		// Starting where the plan in force starts, if that's later than the month
		// in view, so saving without touching it changes nothing.
		const from = goal.monthly_from?.slice(0, 7) ?? '';
		goalMonthlyFrom = from > month.slice(0, 7) ? from : month.slice(0, 7);
		goalName = goal.name;
		goalTarget = String(goal.target_amount);
		goalDate = goal.target_date ?? '';
		goalAccountId = goal.account_id ?? 0;
		goalNotes = goal.notes ?? '';
		goalMonthly = goal.monthly_amount != null ? String(goal.monthly_amount) : '';
		goalError = null;
		goalModalOpen = true;
	}

	function buildGoal(): GoalInput {
		return {
			name: goalName.trim(),
			target_amount: Math.abs(parseFloat(goalTarget) || 0),
			target_date: goalDate || null,
			account_id: goalAccountId || null,
			notes: goalNotes.trim() || null,
			month: goalMonth ? goalMonth.slice(0, 7) : null,
			monthly_amount:
				!goalMonth && parseFloat(goalMonthly) > 0 ? Math.abs(parseFloat(goalMonthly)) : null,
			monthly_from: goalMonth ? null : goalMonthlyFrom || null
		};
	}

	// A monthly goal is edited from its line; the line doesn't carry the whole
	// goal (its notes), so fetch it first.
	async function editMonthlyGoal(goalId: number) {
		try {
			openGoalEdit(await getGoal(goalId));
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the goal';
		}
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

	function openContribute(goal: Pick<Goal, 'id' | 'name' | 'account_id'>, suggested = 0) {
		contribGoal = goal;
		contribAmount = suggested > 0 ? suggested.toFixed(2) : '';
		// From the income account, unless that's where the goal's money lives.
		contribFromId =
			incomeAccountId && incomeAccountId !== goal.account_id
				? incomeAccountId
				: (accounts.find((a) => a.id !== goal.account_id)?.id ?? 0);
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
			const amount = Math.abs(parseFloat(contribAmount) || 0);
			if (contribGoal.account_id) {
				// A transfer into the goal's account is what counts toward it — this
				// month's line and the goal overall — so the money really moves.
				await createTransfer({
					from_account_id: contribFromId,
					to_account_id: contribGoal.account_id,
					amount,
					date: contribDate,
					description: `Savings: ${contribGoal.name}`,
					notes: null
				});
			} else {
				await contributeToGoal(contribGoal.id, amount, contribDate);
			}
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

	// "Oct" for a month's first day, for the "Oct only" tag on a monthly goal.
	const monthShort = (m: string) =>
		new Date(m + 'T00:00:00').toLocaleDateString('en-US', { month: 'short' });

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
				<span class="head-actions">
					<Button
						variant="secondary"
						size="sm"
						onclick={() => openBudget('income')}
						disabled={incomeCategories.length === 0}
					>
						Expect Income
					</Button>
					<Button size="sm" onclick={() => openBudget('expense')} disabled={expenseCategories.length === 0}>
						Set a Budget
					</Button>
				</span>
			</div>

			<div class="month-nav">
				<Button variant="ghost" size="sm" onclick={() => goToMonth(shiftMonth(month, -1))}>
					‹ <span class="neighbor">{formatMonth(shiftMonth(month, -1))}</span>
				</Button>
				<strong class="month-label" class:loading={budgetsLoading}>{formatMonth(month)}</strong>
				<Button variant="ghost" size="sm" onclick={() => goToMonth(shiftMonth(month, 1))}>
					<span class="neighbor">{formatMonth(shiftMonth(month, 1))}</span> ›
				</Button>
				{#if !isCurrentMonth}
					<Button variant="secondary" size="sm" onclick={() => goToMonth(monthStart())}>
						This month
					</Button>
				{/if}
			</div>

			{#if showPlan}
				<div class="plan">
					<div>
						<span class="plan-label">Expected income</span>
						<span class="plan-value">{formatCurrency(totalExpected)}</span>
						<span class="muted small">{formatCurrency(incomeReceived)} received</span>
					</div>
					<span class="plan-op" aria-hidden="true">−</span>
					<div>
						<span class="plan-label">Spending</span>
						<span class="plan-value">{formatCurrency(totalBudget)}</span>
						<span class="muted small">{formatCurrency(totalSpent + unbudgeted)} spent</span>
					</div>
					<span class="plan-op" aria-hidden="true">−</span>
					<div>
						<span class="plan-label">Savings</span>
						<span class="plan-value">{formatCurrency(totalSavingsPlanned)}</span>
						<span class="muted small">
							{formatCurrency(Math.max(0, totalMoved - shortfall))} saved
							{#if shortfall > 0.005}·&nbsp;<span class="neg">{formatCurrency(shortfall)} spent</span>{/if}
						</span>
					</div>
					<span class="plan-op" aria-hidden="true">=</span>
					<div>
						<span class="plan-label">{planLeft >= 0 ? 'Left over' : 'Short'}</span>
						<span class="plan-value" class:pos={planLeft > 0} class:neg={planLeft < 0}>
							{formatCurrency(Math.abs(planLeft))}
						</span>
						<span class="muted small">
							{formatCurrency(incomeReceived - totalSpent - unbudgeted - totalMoved)} actually left
						</span>
					</div>
				</div>
			{/if}

			<label class="income-account">
				<span>Income lands in</span>
				<select value={incomeAccountId} onchange={(e) => chooseIncomeAccount(Number(e.currentTarget.value))}>
					<option value={0}>Choose an account…</option>
					<AccountOptions {accounts} />
				</select>
			</label>

			{#if incomeBudgets.length > 0}
				<h3 class="sub-head">Expected income</h3>
				<div class="list">
					{#each incomeBudgets as budget (budget.id)}
						<!-- Beyond what's already scheduled, so a recurring paycheck isn't counted twice. -->
						{@const toCome = budget.amount - budget.spent - budget.scheduled}
						<div class="item">
							<div class="item-head">
								<span class="item-name">{budget.category_name}</span>
								<span class="item-actions">
									<Button variant="ghost" size="sm" onclick={() => editBudget(budget)}>Edit</Button>
									<Button variant="ghost" size="sm" onclick={() => endBudget(budget)}>Stop</Button>
								</span>
							</div>
							<BudgetBar
								spent={budget.spent}
								amount={budget.amount}
								scheduled={budget.scheduled}
								color="var(--pos)"
								pace={incomePace(month)}
							/>
							<div class="item-foot muted">
								{formatCurrency(budget.spent)} of {formatCurrency(budget.amount)} received
								{#if budget.scheduled > 0}
									· {formatCurrency(budget.scheduled)} scheduled
								{/if}
								{#if toCome > 0.005}
									· {formatCurrency(toCome)} still to come
								{:else if budget.spent > budget.amount + 0.005}
									· <span class="pos">{formatCurrency(budget.spent - budget.amount)} more than expected</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
				{#if unplannedIncome > 0.005}
					<p class="muted small">Plus {formatCurrency(unplannedIncome)} of income you didn't plan for.</p>
				{/if}
			{/if}

			<div class="sub-row">
				<h3 class="sub-head">Savings</h3>
				<Button variant="ghost" size="sm" onclick={() => openGoalCreate(month)}>
					+ Add a monthly goal
				</Button>
			</div>
			{#if savings.length === 0}
				<p class="muted small">
					Nothing set aside for {formatMonth(month)} yet. Add a goal just for this month, or give a
					long-term goal a monthly amount.
				</p>
			{/if}
			{#if savings.length > 0}
				{#if shortfall > 0.005}
					<p class="muted small shortfall">
						More went out of {accounts.find((a) => a.id === incomeAccountId)?.name ?? 'the income account'}
						than this month's income less the savings planned, so
						<strong class="neg">{formatCurrency(shortfall)}</strong> of this month's savings was spent. It's
						split across the goals by their monthly amounts.
					</p>
				{/if}
				<div class="list">
					{#each savings as line (line.goal_id)}
						{@const toGo = line.monthly_amount - line.moved}
						<div class="item">
							<div class="item-head">
								<span class="item-name">
									{line.name}
									{#if line.month}<span class="chip">{monthShort(line.month)} only</span>{/if}
									{#if line.account_name}<span class="muted small">→ {line.account_name}</span>{/if}
								</span>
								<span class="item-actions">
									{#if line.month}
										<Button variant="ghost" size="sm" onclick={() => editMonthlyGoal(line.goal_id)}>
											Edit
										</Button>
									{/if}
									<Button
										variant="ghost"
										size="sm"
										onclick={() =>
											openContribute(
												{ id: line.goal_id, name: line.name, account_id: line.account_id },
												Math.max(0, toGo)
											)}
									>
										Add money
									</Button>
								</span>
							</div>
							<BudgetBar
								spent={line.moved}
								amount={line.monthly_amount}
								color="var(--info)"
								pace={incomePace(month)}
							/>
							<div class="item-foot muted">
								{formatCurrency(line.moved)} of {formatCurrency(line.monthly_amount)} this month
								{#if line.overspent > 0.005}
									· <span class="neg">−{formatCurrency(line.overspent)} spent from savings</span>
									{#if line.moved > 0.005}= {formatCurrency(Math.max(0, line.moved - line.overspent))} saved{/if}
								{/if}
								{#if toGo > 0.005}· {formatCurrency(toGo)} to go{/if}
								{#if !line.month}
									· {formatCurrency(line.saved)} of {formatCurrency(line.target_amount)} overall
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}

			{#if spendBudgets.length > 0}
				<h3 class="sub-head">Spending</h3>
			{/if}

			{#if spendBudgets.length === 0}
				<p class="muted small">
					{#if isCurrentMonth}
						{incomeBudgets.length > 0 ? 'No spending budgets yet.' : 'No budgets set.'} Pick a category
						and a monthly limit to track spending against it, or <strong>Expect Income</strong> to plan
						what comes in.
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
						spent
						{#if totalScheduled > 0}
							+ {formatCurrency(totalScheduled)} scheduled
						{/if}
						of {formatCurrency(totalBudget)} budgeted
						{#if unbudgeted > 0}
							· {formatCurrency(unbudgeted)} unbudgeted
						{/if}
					</span>
				</div>

				<div class="list">
					{#each spendBudgets as budget (budget.id)}
						{@const pace = budgetPace(budget, month)}
						{@const remaining = budget.amount - budget.spent - budget.scheduled}
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
								scheduled={budget.scheduled}
								color={budget.category_color}
								{pace}
							/>
							<div class="item-foot muted">
								{formatCurrency(budget.spent)} of {formatCurrency(budget.amount)}
								{#if budget.scheduled > 0}
									· {formatCurrency(budget.scheduled)} scheduled
								{/if}
								{#if pace.status === 'over'}
									·
									<span class="neg">
										{formatCurrency(budget.spent - budget.amount)} over
									</span>
								{:else if pace.status === 'committed'}
									·
									<span class="warn-text">
										{formatCurrency(-remaining)} over once scheduled charges post
									</span>
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
					{#if totalScheduled > 0 || incomeScheduled > 0}
						Striped: recurring charges{incomeScheduled > 0 ? ' and income' : ''} scheduled but not posted yet.
					{/if}
					Stopping a budget ends it from {formatMonth(month)} on. Earlier months keep it.
				</p>
			{/if}
		</section>

		<section class="card">
			<div class="section-head">
				<h2>Long-term Goals</h2>
				<Button size="sm" onclick={() => openGoalCreate()}>Add a Goal</Button>
			</div>

			{#if goals.length === 0}
				<p class="muted small">
					No long-term goals yet. One is how much you want to add to an account over time, with an
					optional share of each month's budget. Goals for a single month live in that month's
					Savings above.
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
									{#if !goal.achieved}
										<Button variant="ghost" size="sm" onclick={() => openContribute(goal)}>
											Add money
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
								{formatCurrency(goal.saved)} of {formatCurrency(goal.target_amount)} added
								{#if goal.account_name}to {goal.account_name}{/if}
								since {formatDate(goal.started_on)}
								{#if goal.monthly_amount}
									· {formatCurrency(goal.monthly_amount)}/mo{#if goal.monthly_from && goal.monthly_from > month}
										from {formatMonth(goal.monthly_from)}{/if}
								{/if}
								{#if goal.target_date}· by {formatDate(goal.target_date)}{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/if}
</div>

<Modal
	bind:open={budgetModalOpen}
	title="{budgetKind === 'income' ? 'Expected Income' : 'Set a Budget'} for {formatMonth(month)}"
>
	<form onsubmit={saveBudget}>
		<Field label={budgetKind === 'income' ? 'Income category' : 'Category'} id="budgetCategory">
			<select id="budgetCategory" bind:value={budgetCategoryId} disabled={budgetSaving}>
				{#each modalCategories as category (category.id)}
					<option value={category.id}>{category.name}</option>
				{/each}
			</select>
		</Field>

		<Field
			label={budgetKind === 'income' ? 'Expected each month' : 'Monthly limit'}
			id="budgetAmount"
			hint="Applies from {formatMonth(month)} until the next change"
		>
			<input
				id="budgetAmount"
				type="number"
				inputmode="decimal"
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
				{budgetSaving ? 'Saving…' : budgetKind === 'income' ? 'Save Expected Income' : 'Save Budget'}
			</Button>
		</div>
	</form>
</Modal>

<Modal
	bind:open={goalModalOpen}
	title={goalMonth
		? `${editingGoal ? 'Edit' : 'Add'} Goal for ${formatMonth(goalMonth)}`
		: editingGoal
			? 'Edit Goal'
			: 'Add a Long-term Goal'}
>
	<form onsubmit={saveGoal}>
		<Field label="Goal name" id="goalName">
			<input
				id="goalName"
				bind:value={goalName}
				placeholder={goalMonth ? "Refill Zion's" : 'Emergency fund'}
				disabled={goalSaving}
				required
			/>
		</Field>

		<div class="form-row">
			<Field
				label={goalMonth ? 'Amount this month' : 'Amount to add'}
				id="goalTarget"
				hint={goalMonth ? `Only ${formatMonth(goalMonth)} counts` : "On top of what's there today"}
			>
				<input
					id="goalTarget"
					type="number"
					inputmode="decimal"
					step="0.01"
					min="0.01"
					placeholder="0.00"
					bind:value={goalTarget}
					disabled={goalSaving}
					required
				/>
			</Field>
			{#if !goalMonth}
				<Field label="Target date" id="goalDate" hint="Optional">
					<input id="goalDate" type="date" bind:value={goalDate} disabled={goalSaving} />
				</Field>
			{/if}
		</div>

		{#if !goalMonth}
			<div class="form-row">
				<Field label="Each month" id="goalMonthly" hint="Optional — its share of the monthly budget">
					<input
						id="goalMonthly"
						type="number"
						inputmode="decimal"
						step="0.01"
						min="0"
						placeholder="0.00"
						bind:value={goalMonthly}
						disabled={goalSaving}
					/>
				</Field>
				<Field label="Starting" id="goalMonthlyFrom" hint="Applies from this month until you change it">
					<input id="goalMonthlyFrom" type="month" bind:value={goalMonthlyFrom} disabled={goalSaving} />
				</Field>
			</div>
		{/if}

		<Field
			label="Saving into"
			id="goalAccount"
			hint={goalMonth
				? `Money added to this account in ${formatMonth(goalMonth)} counts toward it. Leave unset to log it by hand.`
				: 'Money added to this account counts toward the goal. Leave unset to log it by hand.'}
		>
			<select id="goalAccount" bind:value={goalAccountId} disabled={goalSaving}>
				<option value={0}>Track manually</option>
				<AccountOptions {accounts} />
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
		{#if contribGoal?.account_id}
			<Field
				label="From"
				id="contribFrom"
				hint="Moved as a transfer into {accounts.find((a) => a.id === contribGoal?.account_id)?.name ??
					'the goal account'}"
			>
				<select id="contribFrom" bind:value={contribFromId} disabled={contribSaving} required>
					<AccountOptions accounts={accounts.filter((a) => a.id !== contribGoal?.account_id)} />
				</select>
			</Field>
		{/if}
		<div class="form-row">
			<Field label="Amount" id="contribAmount">
				<input
					id="contribAmount"
					type="number"
					inputmode="decimal"
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
			<Button
				type="submit"
				disabled={contribSaving || !contribAmount || (!!contribGoal?.account_id && !contribFromId)}
			>
				{contribSaving ? 'Saving…' : contribGoal?.account_id ? 'Move money' : 'Add'}
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

	.head-actions {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		justify-content: flex-end;
	}

	/* Expected income − budgeted spending = what's left, as a sum. */
	.plan {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto minmax(0, 1fr) auto minmax(0, 1fr);
		align-items: center;
		gap: 0.75rem;
		padding: 1rem;
		margin-bottom: 1.25rem;
		background: var(--bg);
		border-radius: var(--radius-sm);
	}

	.plan > div {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.plan-label {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--muted);
	}

	.plan-value {
		font-size: 1.25rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.plan-op {
		font-size: 1.25rem;
		color: var(--muted-light);
	}

	.pos {
		color: var(--pos);
	}

	.income-account {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.5rem;
		font-size: 0.875rem;
		color: var(--muted);
	}

	.income-account select {
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		font: inherit;
		background: var(--surface);
		color: var(--ink);
	}

	.shortfall {
		margin: 0 0 0.5rem;
	}

	.sub-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		margin-top: 1.25rem;
	}

	.sub-row .sub-head {
		margin: 0;
	}

	.sub-head {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin: 1.25rem 0 0.5rem;
	}

	@media (max-width: 639px) {
		.plan {
			grid-template-columns: 1fr;
			gap: 0.5rem;
		}

		.plan-op {
			display: none;
		}
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

	@media (max-width: 639px) {
		/* Arrows either side of the month, like a calendar header. */
		.month-nav {
			justify-content: space-between;
			flex-wrap: nowrap;
			background: var(--bg);
			border-radius: var(--radius-sm);
			padding: 0.125rem;
		}

		.neighbor {
			display: none;
		}

		.month-label {
			min-width: 0;
			flex: 1;
		}

		.month-nav :global(.btn) {
			font-size: 1.25rem;
			line-height: 1;
			min-width: 44px;
			min-height: 40px;
		}

		.month-nav :global(.btn.secondary) {
			font-size: 0.75rem;
		}

		.item-head {
			flex-wrap: wrap;
			row-gap: 0.25rem;
		}

		.item-actions {
			margin-left: auto;
			margin-right: -0.5rem;
		}
	}
</style>
