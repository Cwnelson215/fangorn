<script lang="ts">
	import { onMount } from 'svelte';
	import { getDashboard, syncAll } from '$lib/api';
	import type { Dashboard } from '$lib/types';
	import SpendingChart from '$lib/components/SpendingChart.svelte';
	import NetWorthChart from '$lib/components/NetWorthChart.svelte';

	let dashboard: Dashboard | null = $state(null);
	let loading = $state(true);
	let syncing = $state(false);
	let error: string | null = $state(null);

	async function load() {
		try {
			loading = true;
			error = null;
			dashboard = await getDashboard();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load dashboard';
		} finally {
			loading = false;
		}
	}

	async function handleSync() {
		try {
			syncing = true;
			await syncAll();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Sync failed';
		} finally {
			syncing = false;
		}
	}

	onMount(load);

	function formatCurrency(n: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n);
	}
</script>

<svelte:head>
	<title>Fangorn - Dashboard</title>
</svelte:head>

<div class="dashboard">
	<div class="header">
		<h1>Dashboard</h1>
		<button onclick={handleSync} disabled={syncing}>
			{syncing ? 'Syncing...' : 'Sync Transactions'}
		</button>
	</div>

	{#if loading}
		<p class="status">Loading...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if dashboard}
		<div class="summary-cards">
			<div class="card income">
				<div class="card-label">Income</div>
				<div class="card-value">{formatCurrency(dashboard.income)}</div>
			</div>
			<div class="card expenses">
				<div class="card-label">Expenses</div>
				<div class="card-value">{formatCurrency(dashboard.expenses)}</div>
			</div>
			<div class="card net">
				<div class="card-label">Net</div>
				<div class="card-value" class:positive={dashboard.net >= 0} class:negative={dashboard.net < 0}>
					{formatCurrency(dashboard.net)}
				</div>
			</div>
			<div class="card networth">
				<div class="card-label">Net Worth</div>
				<div class="card-value">
					{dashboard.net_worth !== null ? formatCurrency(dashboard.net_worth) : '--'}
				</div>
			</div>
		</div>

		<div class="charts">
			{#if dashboard.categories.length > 0}
				<div class="chart-container">
					<h2>Spending by Category</h2>
					<SpendingChart data={dashboard.categories} />
				</div>
			{/if}

			{#if dashboard.net_worth_history.length > 1}
				<div class="chart-container">
					<h2>Net Worth Over Time</h2>
					<NetWorthChart data={dashboard.net_worth_history} />
				</div>
			{/if}
		</div>

		<div class="date-range">
			Showing {dashboard.from} to {dashboard.to}
		</div>
	{:else}
		<div class="empty">
			<h2>No data yet</h2>
			<p>Link a bank account to get started.</p>
			<a href="/link" class="btn">Link Account</a>
		</div>
	{/if}
</div>

<style>
	.dashboard {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	h1 {
		font-size: 1.75rem;
		font-weight: 700;
	}

	button {
		background: #4ecca3;
		color: #1a1a2e;
		border: none;
		padding: 0.5rem 1.25rem;
		border-radius: 8px;
		font-weight: 600;
		cursor: pointer;
		font-size: 0.9rem;
	}

	button:hover {
		background: #3db88f;
	}

	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.summary-cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 1rem;
	}

	.card {
		background: white;
		border-radius: 12px;
		padding: 1.25rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	.card-label {
		font-size: 0.85rem;
		color: #666;
		font-weight: 500;
		margin-bottom: 0.25rem;
	}

	.card-value {
		font-size: 1.5rem;
		font-weight: 700;
	}

	.positive {
		color: #22c55e;
	}

	.negative {
		color: #ef4444;
	}

	.charts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
		gap: 1.5rem;
	}

	.chart-container {
		background: white;
		border-radius: 12px;
		padding: 1.5rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	.chart-container h2 {
		font-size: 1.1rem;
		margin-bottom: 1rem;
	}

	.date-range {
		text-align: center;
		color: #999;
		font-size: 0.85rem;
	}

	.status {
		text-align: center;
		padding: 3rem;
		color: #666;
	}

	.status.error {
		color: #ef4444;
	}

	.empty {
		text-align: center;
		padding: 4rem 2rem;
		background: white;
		border-radius: 12px;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	.empty h2 {
		margin-bottom: 0.5rem;
	}

	.empty p {
		color: #666;
		margin-bottom: 1.5rem;
	}

	.btn {
		display: inline-block;
		background: #4ecca3;
		color: #1a1a2e;
		text-decoration: none;
		padding: 0.75rem 2rem;
		border-radius: 8px;
		font-weight: 600;
	}
</style>
