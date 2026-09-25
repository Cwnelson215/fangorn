<script lang="ts">
	import { goto } from '$app/navigation';
	import { login } from '$lib/api';

	let password = $state('');
	let error: string | null = $state(null);
	let loading = $state(false);

	async function handleLogin(e: Event) {
		e.preventDefault();
		loading = true;
		error = null;

		try {
			await login(password);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
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
		min-height: 100dvh;
		padding: 1rem;
		padding-top: max(1rem, env(safe-area-inset-top));
		background: var(--bg);
	}

	.login-card {
		background: var(--surface);
		border: 1px solid var(--divider);
		border-radius: var(--radius);
		padding: 3rem;
		max-width: 400px;
		width: 100%;
		text-align: center;
		box-shadow: var(--shadow-lg);
	}

	h1 {
		font-size: 2rem;
		font-weight: 600;
		color: var(--ink);
		margin-bottom: 0.5rem;
	}

	p {
		color: var(--muted);
		margin-bottom: 1.5rem;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	input {
		padding: 0.75rem 1rem;
		border: 1px solid var(--border);
		border-radius: 10px;
		font-size: 1rem;
		outline: none;
		transition: border-color 0.2s;
	}

	input:focus {
		border-color: var(--accent);
	}

	button {
		background: var(--accent);
		color: var(--on-accent);
		border: none;
		padding: 0.75rem;
		border-radius: 10px;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.2s;
	}

	button:hover {
		background: var(--accent-hover);
	}

	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	@media (max-width: 639px) {
		.login-card {
			padding: 2rem 1.5rem;
		}

		input,
		button {
			min-height: 48px;
		}
	}

	.error {
		color: var(--neg);
		margin-top: 1rem;
		margin-bottom: 0;
	}
</style>
