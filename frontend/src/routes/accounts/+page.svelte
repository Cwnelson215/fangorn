<script lang="ts">
	import { onMount } from 'svelte';
	import { getAccounts } from '$lib/api';
	import type { Account } from '$lib/types';
	import AccountCard from '$lib/components/AccountCard.svelte';

	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	onMount(async () => {
		try {
			accounts = await getAccounts();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load accounts';
		} finally {
			loading = false;
		}
	});

	const grouped = $derived(
		accounts.reduce<Record<string, Account[]>>((acc, account) => {
			const type = account.type;
			if (!acc[type]) acc[type] = [];
			acc[type].push(account);
			return acc;
		}, {})
	);
</script>

<svelte:head>
	<title>Fangorn - Accounts</title>
</svelte:head>

<div class="accounts-page">
	<div class="header">
		<h1>Accounts</h1>
		<a href="/link" class="btn">Link Account</a>
	</div>

	{#if loading}
		<p class="status">Loading...</p>
	{:else if error}
		<p class="status error">{error}</p>
	{:else if accounts.length === 0}
		<div class="empty">
			<p>No accounts linked yet.</p>
			<a href="/link" class="btn">Link your first account</a>
		</div>
	{:else}
		{#each Object.entries(grouped) as [type, typeAccounts]}
			<section>
				<h2>{type.charAt(0).toUpperCase() + type.slice(1)}</h2>
				<div class="account-grid">
					{#each typeAccounts as account}
						<AccountCard {account} />
					{/each}
				</div>
			</section>
		{/each}
	{/if}
</div>

<style>
	.accounts-page {
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

	h2 {
		font-size: 1.1rem;
		font-weight: 600;
		text-transform: capitalize;
		margin-bottom: 0.75rem;
		color: #666;
	}

	.account-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 1rem;
	}

	.btn {
		display: inline-block;
		background: #4ecca3;
		color: #1a1a2e;
		text-decoration: none;
		padding: 0.5rem 1.25rem;
		border-radius: 8px;
		font-weight: 600;
		font-size: 0.9rem;
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
		padding: 3rem;
		background: white;
		border-radius: 12px;
	}

	.empty p {
		color: #666;
		margin-bottom: 1rem;
	}
</style>
