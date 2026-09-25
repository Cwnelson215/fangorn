<script lang="ts">
	// What if: pick an account — or nothing — and play with everything about it.
	// How much is in it, what goes in each month and how that grows, the return,
	// a lump sum in or out. The account as it really is (its balance, its
	// recurring transfers, its usual rate) stays on the chart as "as things are",
	// so every change shows as a difference. Nothing here is saved; reload and it
	// starts from the account again. A card or loan gets the payoff view instead.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getAccountValueHistory, getAccounts, getRules } from '$lib/api';
	import type { Account, RecurringRule } from '$lib/types';
	import { accountKindLabel, holdsSecurities } from '$lib/types';
	import { formatCurrencyWhole, today } from '$lib/format';
	import { addMonths, formatDuration, monthlyInflow, projectGrowth } from '$lib/projection';
	import { SERIES } from '$lib/chart';
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	import ProjectionChart from '$lib/components/ProjectionChart.svelte';
	import PayoffCard from '$lib/components/PayoffCard.svelte';
	import SliderField from '$lib/components/SliderField.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

	const INFLATION = 0.025;

	let accounts = $state<Account[]>([]);
	let rules = $state<RecurringRule[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// 0 is "start from scratch".
	let accountId = $state(0);
	let scratchKind = $state<'grow' | 'payoff'>('grow');

	let account = $derived(accounts.find((a) => a.id === accountId));
	let isDebt = $derived(account ? account.class === 'liability' : scratchKind === 'payoff');
	// What's really there: an asset's balance, or what a card or loan owes.
	let actual = $derived(account ? (account.class === 'liability' ? Math.max(0, -account.balance) : account.balance) : 0);
	let inflow = $derived(account ? monthlyInflow(rules, account.id) : 0);
	let usualRate = $derived(rateFor(account));

	// The assumptions, all of them the user's to change.
	let balance = $state(0);
	let monthly = $state(0);
	let raise = $state(0);
	let rate = $state(7);
	let years = $state(10);
	let lumpAmount = $state(0);
	let lumpYear = $state(5);
	let real = $state(false);

	function rateFor(a: Account | undefined): number {
		if (!a) return 7;
		if (a.type === 'high_yield_savings') return a.apy ?? 4;
		if (holdsSecurities(a.type)) return 7;
		return 0;
	}

	// Back to the account as it is.
	function reset() {
		// Exact, not rounded: 33¢ a month compounds into hundreds over decades,
		// and an untouched what-if has to match "as things are" to the cent.
		balance = actual;
		monthly = inflow;
		raise = 0;
		rate = usualRate;
		years = account?.type === 'retirement' ? 25 : account?.type === 'high_yield_savings' ? 5 : 10;
		lumpAmount = 0;
		lumpYear = Math.max(1, Math.min(5, years));
		real = false;
	}

	function choose(id: number) {
		accountId = id;
		reset();
		goto(id ? `/whatif?account=${id}` : '/whatif', { replaceState: true, noScroll: true, keepFocus: true });
	}

	onMount(async () => {
		try {
			[accounts, rules] = await Promise.all([getAccounts(), getRules().catch(() => [])]);
			const wanted = Number(page.url.searchParams.get('account'));
			accountId = accounts.some((a) => a.id === wanted) ? wanted : 0;
			reset();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load your accounts';
		} finally {
			loading = false;
		}
	});

	let history = $state<{ date: string; value: number }[]>([]);
	$effect(() => {
		const id = accountId;
		history = [];
		if (!id || isDebt) return;
		let stale = false;
		getAccountValueHistory(id, 365)
			.then((pts) => {
				if (!stale) history = pts.map((p) => ({ date: p.date, value: p.value }));
			})
			.catch(() => {});
		return () => {
			stale = true;
		};
	});

	let now = today();
	let months = $derived(Math.max(1, Math.round(years * 12)));
	let inflation = $derived(real ? INFLATION : 0);

	let whatIf = $derived(
		projectGrowth({
			start: balance,
			monthly,
			annualReturn: rate / 100,
			months,
			inflation,
			raise: raise / 100,
			oneTime: lumpAmount !== 0 ? [{ month: Math.min(months, Math.round(lumpYear * 12)), amount: lumpAmount }] : []
		})
	);
	// With no account there's nothing "as things are" to compare against.
	let asIs = $derived(
		account ? projectGrowth({ start: actual, monthly: inflow, annualReturn: usualRate / 100, months, inflation }) : null
	);

	let rows = $derived(
		whatIf.map((p, i) => ({
			date: addMonths(now, p.month),
			contributed: p.contributed,
			growth: p.growth,
			asIs: asIs?.[i].balance ?? 0
		}))
	);
	const stack = [
		{ key: 'contributed', label: 'Put in', color: SERIES.blue },
		{ key: 'growth', label: 'Growth', color: SERIES.green }
	];
	let lines = $derived(asIs ? [{ key: 'asIs', label: 'As things are', color: SERIES.orange }] : []);

	let end = $derived(whatIf[whatIf.length - 1]);
	let asIsEnd = $derived(asIs ? asIs[asIs.length - 1] : null);
	let difference = $derived(asIsEnd ? end.balance - asIsEnd.balance : null);
	let changed = $derived(
		account != null &&
			(actual !== balance ||
				inflow !== monthly ||
				raise !== 0 ||
				rate !== usualRate ||
				lumpAmount !== 0)
	);

	// One row per year for the table under the chart.
	let byYear = $derived(
		whatIf
			.filter((p) => p.month > 0 && (p.month % 12 === 0 || p.month === months))
			.map((p) => ({ ...p, asIs: asIs?.[p.month].balance ?? null }))
	);
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>What if</h1>
			<p class="muted">Pick an account and change anything about it. Nothing here is saved.</p>
		</div>
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if error}
		<p class="error-text">{error}</p>
	{:else}
		<div class="card start">
			<div class="form-row">
				<Field label="Start from" id="whatifAccount">
					<select id="whatifAccount" value={accountId} onchange={(e) => choose(Number(e.currentTarget.value))}>
						<option value={0}>Nothing — start from scratch</option>
						<AccountOptions {accounts} />
					</select>
				</Field>
				{#if !account}
					<Field label="Kind" id="whatifKind">
						<select id="whatifKind" bind:value={scratchKind}>
							<option value="grow">Money that grows</option>
							<option value="payoff">A debt to pay off</option>
						</select>
					</Field>
				{/if}
			</div>
			{#if account}
				<p class="muted small">
					{accountKindLabel(account)} · really {isDebt ? 'owes' : 'has'} {formatCurrencyWhole(actual)}
					{#if inflow > 0}· recurring transfers add {formatCurrencyWhole(inflow)}/mo{/if}
					· <a href="/accounts/{account.id}">open account</a>
				</p>
			{/if}
		</div>

		{#if isDebt}
			<div class="card">
				<SliderField
					label="Owed today"
					id="whatifOwed"
					bind:value={balance}
					min={0}
					max={Math.max(10_000, Math.ceil((actual * 2) / 1000) * 1000)}
					step={50}
					prefix="$"
					hint={account ? `Really ${formatCurrencyWhole(actual)}` : undefined}
				/>
			</div>
			{#key accountId}
				<PayoffCard
					id="whatif-{accountId || 'new'}"
					kind={account?.type === 'credit_card' ? 'credit_card' : 'loan'}
					owed={balance}
					defaultPayment={inflow}
					loadHistory={account ? () => getAccountValueHistory(account.id, 365) : undefined}
					remember={false}
				/>
			{/key}
		{:else}
			<div class="card">
				<div class="stats">
					<div>
						<span class="label">What if · in {formatDuration(months)}</span>
						<span class="value">{formatCurrencyWhole(end.balance)}</span>
						<span class="muted small">
							{formatCurrencyWhole(end.contributed)} put in · {formatCurrencyWhole(end.growth)} growth
						</span>
					</div>
					{#if asIsEnd}
						<div>
							<span class="label">As things are</span>
							<span class="value">{formatCurrencyWhole(asIsEnd.balance)}</span>
							<span class="muted small">
								{formatCurrencyWhole(actual)} at {usualRate}%, adding {formatCurrencyWhole(inflow)}/mo
							</span>
						</div>
						<div>
							<span class="label">Difference</span>
							<span class="value" class:pos={(difference ?? 0) > 0.5} class:neg={(difference ?? 0) < -0.5}>
								{(difference ?? 0) < 0 ? '−' : '+'}{formatCurrencyWhole(Math.abs(difference ?? 0))}
							</span>
							<span class="muted small">{changed ? 'from your changes' : 'change something below'}</span>
						</div>
					{/if}
				</div>

				<ProjectionChart {history} historyLabel="Actual so far" {rows} {stack} {lines} height={320} />
			</div>

			<div class="card">
				<div class="card-head">
					<h2>Assumptions</h2>
					{#if account}<Button variant="ghost" size="sm" onclick={reset}>Back to actual</Button>{/if}
				</div>
				<div class="controls">
					<SliderField
						label="In it today"
						id="whatifBalance"
						bind:value={balance}
						min={0}
						max={Math.max(50_000, Math.ceil((actual * 3) / 1000) * 1000)}
						step={100}
						prefix="$"
						hint={account ? `Really ${formatCurrencyWhole(actual)}` : undefined}
					/>
					<SliderField
						label="Added each month"
						id="whatifMonthly"
						bind:value={monthly}
						min={0}
						max={Math.max(3000, Math.ceil((inflow * 3) / 100) * 100)}
						step={25}
						prefix="$"
						hint={inflow > 0 ? `Recurring transfers add ${formatCurrencyWhole(inflow)}` : undefined}
					/>
					<SliderField
						label="Raise it each year by"
						id="whatifRaise"
						bind:value={raise}
						min={0}
						max={10}
						step={0.5}
						suffix="%"
						hint="Contributions grow as pay does"
					/>
					<SliderField
						label="Yearly return"
						id="whatifRate"
						bind:value={rate}
						min={0}
						max={15}
						step={0.25}
						suffix="%"
						hint={account ? `Usually ${usualRate}% for this account` : 'Stocks have averaged roughly 10%, about 7% after inflation'}
					/>
					<SliderField label="Years" id="whatifYears" bind:value={years} min={1} max={45} step={1} suffix="yr" />
					<div class="lump">
						<SliderField
							label="One-time deposit (or − withdrawal)"
							id="whatifLump"
							bind:value={lumpAmount}
							min={-Math.max(10_000, Math.ceil(actual / 1000) * 1000)}
							max={Math.max(10_000, Math.ceil(actual / 1000) * 1000)}
							step={100}
							prefix="$"
						/>
						{#if lumpAmount !== 0}
							<SliderField
								label="In year"
								id="whatifLumpYear"
								bind:value={lumpYear}
								min={1}
								max={years}
								step={1}
								hint={`Around ${addMonths(now, Math.round(lumpYear * 12)).slice(0, 7)}`}
							/>
						{/if}
					</div>
					<label class="toggle">
						<input type="checkbox" bind:checked={real} />
						<span>
							Show in today's dollars <span class="muted small">(less {INFLATION * 100}% inflation a year)</span>
						</span>
					</label>
				</div>
			</div>

			<div class="card">
				<details>
					<summary>Year by year</summary>
					<div class="table-scroll">
						<table>
							<thead>
								<tr>
									<th>Year</th>
									<th class="right">What if</th>
									<th class="right">Put in</th>
									<th class="right">Growth</th>
									{#if asIs}<th class="right">As things are</th>{/if}
								</tr>
							</thead>
							<tbody>
								{#each byYear as y (y.month)}
									<tr>
										<td>{addMonths(now, y.month).slice(0, 4)} <span class="muted small">({formatDuration(y.month)})</span></td>
										<td class="right num">{formatCurrencyWhole(y.balance)}</td>
										<td class="right num">{formatCurrencyWhole(y.contributed)}</td>
										<td class="right num">{formatCurrencyWhole(y.growth)}</td>
										{#if y.asIs != null}<td class="right num">{formatCurrencyWhole(y.asIs)}</td>{/if}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</details>
			</div>
		{/if}
	{/if}
</div>

<style>
	.page-header p {
		margin: 0.25rem 0 0;
	}

	.start .muted.small {
		margin: 0.75rem 0 0;
	}

	.start a {
		color: var(--info);
		font-weight: 600;
		text-decoration: none;
	}

	.small {
		font-size: 0.75rem;
	}

	h2 {
		font-size: 1rem;
	}

	.card-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.stats {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.stats > div {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.label {
		font-size: 0.8125rem;
		color: var(--muted);
		font-weight: 500;
	}

	.value {
		font-size: 1.5rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.value.pos {
		color: var(--pos);
	}

	.value.neg {
		color: var(--neg);
	}

	.controls {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1.25rem 1.5rem;
	}

	.lump {
		grid-column: span 2;
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1.5rem;
	}

	.toggle {
		grid-column: 1 / -1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		cursor: pointer;
	}

	summary {
		cursor: pointer;
		font-weight: 600;
	}

	.table-scroll {
		overflow-x: auto;
		margin-top: 0.75rem;
	}

	table {
		width: 100%;
		min-width: 480px;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	th {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		text-align: left;
		padding: 0.5rem;
		background: var(--bg);
	}

	td {
		padding: 0.5rem;
		border-bottom: 1px solid var(--divider);
	}

	.right {
		text-align: right;
	}

	.num {
		font-variant-numeric: tabular-nums;
	}

	@media (max-width: 639px) {
		.stats {
			grid-template-columns: 1fr 1fr;
		}

		.stats > div:first-child {
			grid-column: 1 / -1;
		}

		.controls,
		.lump {
			grid-template-columns: 1fr;
		}

		.lump {
			grid-column: auto;
			gap: 1.25rem;
		}
	}
</style>
