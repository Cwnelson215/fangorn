<script lang="ts">
	import { linkAccount, getConfig } from '$lib/api';
	import { goto } from '$app/navigation';

	let loading = $state(false);
	let error: string | null = $state(null);
	let success = $state(false);

	async function openTellerConnect() {
		try {
			loading = true;
			error = null;

			const config = await getConfig();

			const teller = (window as any).TellerConnect.setup({
				applicationId: config.teller_app_id,
				onSuccess: async (enrollment: any) => {
					try {
						await linkAccount(
							enrollment.accessToken,
							enrollment.enrollment?.id || '',
							enrollment.enrollment?.institution?.name || ''
						);
						success = true;
						setTimeout(() => goto('/accounts'), 1500);
					} catch (e) {
						error = e instanceof Error ? e.message : 'Failed to link account';
					}
				},
				onExit: () => {
					loading = false;
				},
				onFailure: (failure: any) => {
					loading = false;
					error = failure?.message || 'Connection failed';
				}
			});

			teller.open();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to start Teller Connect';
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Fangorn - Link Account</title>
	<script src="https://cdn.teller.io/connect/connect.js"></script>
</svelte:head>

<div class="link-page">
	<div class="link-card">
		<h1>Link a Bank Account</h1>
		<p>Connect your bank account to automatically sync transactions and track your spending.</p>

		{#if success}
			<div class="success-message">
				Account linked successfully! Redirecting...
			</div>
		{:else}
			<button onclick={openTellerConnect} disabled={loading}>
				{loading ? 'Connecting...' : 'Connect Bank Account'}
			</button>
		{/if}

		{#if error}
			<p class="error">{error}</p>
		{/if}

		<div class="info">
			<p>Your credentials are handled securely by Teller and never stored on our servers.</p>
		</div>
	</div>
</div>

<style>
	.link-page {
		display: flex;
		justify-content: center;
		padding-top: 3rem;
	}

	.link-card {
		background: white;
		border-radius: 16px;
		padding: 3rem;
		max-width: 480px;
		width: 100%;
		text-align: center;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin-bottom: 0.75rem;
	}

	p {
		color: #666;
		margin-bottom: 2rem;
		line-height: 1.6;
	}

	button {
		background: #4ecca3;
		color: #1a1a2e;
		border: none;
		padding: 0.875rem 2.5rem;
		border-radius: 10px;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.2s;
	}

	button:hover {
		background: #3db88f;
	}

	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.success-message {
		background: #dcfce7;
		color: #166534;
		padding: 1rem;
		border-radius: 8px;
		font-weight: 500;
	}

	.error {
		color: #ef4444;
		margin-top: 1rem;
	}

	.info {
		margin-top: 2rem;
		padding-top: 1.5rem;
		border-top: 1px solid #eee;
	}

	.info p {
		font-size: 0.85rem;
		color: #999;
		margin-bottom: 0;
	}
</style>
