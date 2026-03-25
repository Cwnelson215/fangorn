<script lang="ts">
	import { goto } from '$app/navigation';

	let password = $state('');
	let error: string | null = $state(null);
	let loading = $state(false);

	async function handleLogin(e: Event) {
		e.preventDefault();
		loading = true;
		error = null;

		try {
			const res = await fetch('/api/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ password })
			});

			if (!res.ok) {
				const data = await res.json().catch(() => ({ error: 'Login failed' }));
				error = data.error || 'Login failed';
				return;
			}

			goto('/');
		} catch {
			error = 'Connection error';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Fangorn - Login</title>
</svelte:head>

<div class="login-page">
	<div class="login-card">
		<h1>Fangorn</h1>
		<p>Enter your password to continue</p>

		<form onsubmit={handleLogin}>
			<input
				type="password"
				bind:value={password}
				placeholder="Password"
				disabled={loading}
				autocomplete="current-password"
			/>
			<button type="submit" disabled={loading || !password}>
				{loading ? 'Logging in...' : 'Log in'}
			</button>
		</form>

		{#if error}
			<p class="error">{error}</p>
		{/if}
	</div>
</div>

<style>
	.login-page {
		display: flex;
		justify-content: center;
		align-items: center;
		min-height: 100vh;
		background: #f8f9fa;
	}

	.login-card {
		background: white;
		border-radius: 16px;
		padding: 3rem;
		max-width: 400px;
		width: 100%;
		text-align: center;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
	}

	h1 {
		font-size: 1.75rem;
		font-weight: 700;
		color: #4ecca3;
		margin-bottom: 0.5rem;
	}

	p {
		color: #666;
		margin-bottom: 1.5rem;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	input {
		padding: 0.75rem 1rem;
		border: 1px solid #ddd;
		border-radius: 10px;
		font-size: 1rem;
		outline: none;
		transition: border-color 0.2s;
	}

	input:focus {
		border-color: #4ecca3;
	}

	button {
		background: #4ecca3;
		color: #1a1a2e;
		border: none;
		padding: 0.75rem;
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

	.error {
		color: #ef4444;
		margin-top: 1rem;
		margin-bottom: 0;
	}
</style>
