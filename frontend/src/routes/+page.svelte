<script lang="ts">
	import { onMount } from 'svelte';
	import { getDashboard } from '$lib/api';
	import type { Dashboard } from '$lib/types';
	import {
		formatCurrency,
		formatDate,
		formatPercent,
		formatSigned,
		monthStart,
		relativeDays
	} from '$lib/format';
	import { budgetPace } from '$lib/budget';
	import BudgetBar from '$lib/components/BudgetBar.svelte';
	import DonutChart from '$lib/components/DonutChart.svelte';
	import ValueChart from '$lib/components/ValueChart.svelte';
	import TrendChart from '$lib/components/TrendChart.svelte';
	import GroupBySwitch from '$lib/components/GroupBySwitch.svelte';
	import { groupAccounts } from '$lib/grouping';
	import { grouping } from '$lib/grouping.svelte';

	let data = $state<Dashboard | null>(null);
	// Expected income lives on the budgets page; the dashboard tracks spending.
	let spendingBudgets = $derived(data?.budgets.filter((b) => b.kind === 'expense') ?? []);
	let accountGroups = $derived(data ? groupAccounts(data.accounts, grouping.by) : []);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(load);

	async function load() {
		loading = true;
		error = null;
		try {
			data = await getDashboard();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load your dashboard';
		} finally {
			loading = false;
		}
	}

	let hasAccounts = $derived((data?.accounts.length ?? 0) > 0);
	let activeGoals = $derived(data?.goals.filter((g) => !g.achieved) ?? []);

</script>

<div class="page">
	<div class="page-header">
		<h1>Dashboard</h1>
		{#if data}
			<span class="muted">{formatDate(data.from)} – {formatDate(data.to)}</span>
		{/if}
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else if data}
		{#if !hasAccounts}
			<div class="card empty">
				<h2>Let's set up your accounts</h2>
				<p class="muted">
					Add each account with the balance it has today. Every transaction you log from then on
					adjusts it from there.
				</p>
				<a class="cta" href="/accounts">Add your first account</a>
			</div>
		{:else}
			<div class="stats">
				<div class="card stat">
					<span class="stat-label">Net Worth</span>
					<span class="stat-value">{formatCurrency(data.net_worth)}</span>
					<span class="muted">
						{formatCurrency(data.total_assets)} assets · {formatCurrency(data.total_liabilities)} owed
					</span>
					{#if data.retirement_value > 0}
						<span class="muted">
							{formatCurrency(data.retirement_value)} retirement ·
							{formatCurrency(data.net_worth - data.retirement_value)} outside it
						</span>
					{/if}
					{#if data.investments}
						{@const inv = data.investments}
						<a class="investments-line" href="/investments">
							Investments {formatCurrency(inv.total_value)} ·
							<span class:pos={inv.day_change > 0.004} class:neg={inv.day_change < -0.004}>
								{formatSigned(inv.day_change)}
								{#if inv.day_change_pct != null}({formatPercent(inv.day_change_pct, true)}){/if}
							</span>
							today
						</a>
					{/if}
				</div>
				<div class="card stat">
					<span class="stat-label">Money In</span>
					<span class="stat-value pos">{formatCurrency(data.income)}</span>
					<span class="muted">last 30 days</span>
				</div>
				<div class="card stat">
					<span class="stat-label">Money Out</span>
					<span class="stat-value neg">{formatCurrency(data.expenses)}</span>
					<span class="muted">last 30 days</span>
				</div>
				<div class="card stat">
					<span class="stat-label">Net</span>
					<span class="stat-value" class:pos={data.net >= 0} class:neg={data.net < 0}>
						{formatCurrency(data.net)}
					</span>
					<span class="muted">in minus out</span>
				</div>
			</div>

			<div class="card">
				<div class="card-head accounts-head">
					<h2>Accounts</h2>
					<GroupBySwitch />
				</div>
				<div class="account-list">
					{#each accountGroups as group (group.key)}
						<h3 class="group-heading">{group.label}</h3>
						{#each group.items as account (account.id)}
							<a class="account-line" href="/accounts/{account.id}">
								<span class="account-name">{account.name}</span>
								<span
									class="account-balance"
									class:neg={account.class === 'liability' && account.balance !== 0}
								>
									{formatCurrency(
										account.class === 'liability' ? Math.abs(account.balance) : account.balance
									)}
								</span>
							</a>
						{/each}
					{/each}
				</div>
			</div>

			{#if data.upcoming.length > 0}
				<div class="card">
					<div class="card-head">
						<h2>Coming Up</h2>
						<a class="link" href="/recurring">Manage</a>
					</div>
					<div class="upcoming-list">
						{#each data.upcoming.slice(0, 8) as occurrence (occurrence.id)}
							<div class="upcoming-row">
								<span class="upcoming-name">
									{occurrence.rule_name}
									<span class="muted">
										{occurrence.kind === 'transfer'
											? `${occurrence.account_name} → ${occurrence.to_account_name}`
											: occurrence.account_name}
									</span>
								</span>
								<span class="upcoming-when">{relativeDays(occurrence.due_date)}</span>
								<span class="upcoming-amount" class:neg={occurrence.kind === 'expense'}>
									{formatCurrency(occurrence.amount ?? 0)}
								</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<div class="grid-2">
				<div class="card">
					<h2>Net Worth</h2>
					{#if data.net_worth_history.length >= 2}
						<ValueChart
							points={data.net_worth_history.map((p) => ({ date: p.date, value: p.net_worth }))}
							id="dashboard-nw"
						/>
					{:else}
						<p class="muted small">
							History builds up one snapshot a day — check back tomorrow for a trend line.
						</p>
					{/if}
				</div>

				<div class="card">
					<h2>Spending by Category</h2>
					{#if data.categories.length > 0}
						<DonutChart
							slices={data.categories.map((c) => ({ label: c.category_name, value: c.amount, color: c.color }))}
						/>
					{:else}
						<p class="muted small">No spending logged in this period yet.</p>
					{/if}
				</div>
			</div>

			<div class="card">
				<h2>Weekly Spending</h2>
				{#if data.weekly_spending.some((w) => w.amount > 0)}
					<TrendChart data={data.weekly_spending} />
				{:else}
					<p class="muted small">No spending logged in the last 12 weeks.</p>
				{/if}
			</div>

			{#if spendingBudgets.length > 0}
				<div class="card">
					<div class="card-head">
						<h2>This Month's Budgets</h2>
						<a class="link" href="/budgets">Manage</a>
					</div>
					<div class="budget-list">
						{#each spendingBudgets as budget (budget.id)}
							{@const pace = budgetPace(budget, monthStart())}
							<div class="budget">
								<div class="budget-head">
									<span>{budget.category_name}</span>
									<span
										class="muted"
										class:neg={pace.status === 'over'}
										class:warn-text={pace.status === 'committed' || pace.status === 'ahead'}
									>
										{formatCurrency(budget.spent)} of {formatCurrency(budget.amount)}
									</span>
								</div>
								<BudgetBar
									spent={budget.spent}
									amount={budget.amount}
									scheduled={budget.scheduled}
									color={budget.category_color}
									{pace}
								/>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			{#if activeGoals.length > 0}
				<div class="card">
					<div class="card-head">
						<h2>Goals</h2>
						<a class="link" href="/budgets">Manage</a>
					</div>
					<div class="budget-list">
						{#each activeGoals as goal (goal.id)}
							{@const pct = Math.min(100, Math.max(0, (goal.saved / goal.target_amount) * 100))}
							<div class="budget">
								<div class="budget-head">
									<span>{goal.name}</span>
									<span class="muted">
										{formatCurrency(goal.saved)} of {formatCurrency(goal.target_amount)}
									</span>
								</div>
								<div class="bar">
									<div class="bar-fill" style="width: {pct}%; background: var(--info)"></div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	{/if}
</div>

<style>
	h2 {
		font-size: 1rem;
		margin-bottom: 1rem;
	}

	.card-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		margin-bottom: 1rem;
	}

	.card-head h2 {
		margin-bottom: 0;
	}

	.link {
		font-size: 0.8125rem;
		color: var(--muted);
		text-decoration: none;
	}

	.link:hover {
		color: var(--ink);
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 1rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}

	.stat-label {
		font-size: 0.8125rem;
		color: var(--muted);
		font-weight: 500;
	}

	.stat-value {
		font-size: 1.75rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.investments-line {
		font-size: 0.8125rem;
		color: var(--muted);
		text-decoration: none;
	}

	.investments-line:hover {
		color: var(--ink);
	}

	.grid-2 {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(340px, 100%), 1fr));
		gap: 1.5rem;
	}

	.account-list,
	.upcoming-list,
	.budget-list {
		display: flex;
		flex-direction: column;
	}

	.accounts-head {
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.group-heading {
		margin: 0.75rem 0 0;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--muted);
	}

	.group-heading:first-child {
		margin-top: 0;
	}

	.account-line {
		display: flex;
		justify-content: space-between;
		padding: 0.625rem 0;
		border-bottom: 1px solid #f0f0f0;
		text-decoration: none;
		color: inherit;
	}

	.account-line:last-child {
		border-bottom: none;
	}

	.account-line:hover .account-name {
		color: var(--accent-hover);
	}

	.account-balance {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.upcoming-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		gap: 1rem;
		align-items: center;
		padding: 0.625rem 0;
		border-bottom: 1px solid #f0f0f0;
	}

	.upcoming-row:last-child {
		border-bottom: none;
	}

	.upcoming-name {
		display: flex;
		flex-direction: column;
		line-height: 1.3;
	}

	.upcoming-name .muted {
		font-size: 0.75rem;
	}

	.upcoming-when {
		font-size: 0.8125rem;
		color: var(--muted);
	}

	.upcoming-amount {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		min-width: 90px;
		text-align: right;
	}

	.budget {
		padding: 0.625rem 0;
	}

	.budget-head {
		display: flex;
		justify-content: space-between;
		font-size: 0.875rem;
		margin-bottom: 0.375rem;
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

	.empty h2 {
		margin-bottom: 0.5rem;
	}

	.empty .muted {
		max-width: 30rem;
		margin: 0 auto 1.5rem;
	}

	.cta {
		display: inline-block;
		background: var(--accent);
		color: var(--ink);
		padding: 0.625rem 1.25rem;
		border-radius: var(--radius-sm);
		font-weight: 600;
		text-decoration: none;
	}

	.cta:hover {
		background: var(--accent-hover);
	}

	.small {
		font-size: 0.875rem;
	}

	@media (max-width: 639px) {
		/* Net worth gets the full width; the three 30-day figures share a row. */
		.stats {
			grid-template-columns: repeat(3, minmax(0, 1fr));
			gap: 0.5rem;
		}

		.stats .stat:first-child {
			grid-column: 1 / -1;
		}

		.stats .stat:not(:first-child) {
			padding: 0.75rem;
		}

		.stats .stat:not(:first-child) .stat-value {
			font-size: 1rem;
		}

		.stats .stat:not(:first-child) .stat-label,
		.stats .stat:not(:first-child) .muted {
			font-size: 0.6875rem;
		}

		.upcoming-row {
			grid-template-columns: minmax(0, 1fr) auto;
			grid-template-areas:
				'name amount'
				'name when';
			column-gap: 0.75rem;
			row-gap: 0;
		}

		.upcoming-name {
			grid-area: name;
		}

		.upcoming-amount {
			grid-area: amount;
			min-width: 0;
		}

		.upcoming-when {
			grid-area: when;
			text-align: right;
			font-size: 0.75rem;
		}
	}
</style>
