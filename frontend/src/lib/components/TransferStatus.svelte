<script lang="ts">
	type Step = { label: string; state: 'done' | 'active' | 'waiting' | 'error' };

	let { overallStatus }: {
		overallStatus: string;
	} = $props();

	const steps = $derived.by((): Step[] => {
		if (overallStatus === 'failed') {
			return [
				{ label: 'Created', state: 'done' },
				{ label: 'Processing', state: 'error' },
				{ label: 'Complete', state: 'waiting' }
			];
		}
		if (overallStatus === 'cancelled') {
			return [
				{ label: 'Created', state: 'done' },
				{ label: 'Cancelled', state: 'error' },
				{ label: 'Complete', state: 'waiting' }
			];
		}
		if (overallStatus === 'completed') {
			return [
				{ label: 'Created', state: 'done' },
				{ label: 'Processing', state: 'done' },
				{ label: 'Complete', state: 'done' }
			];
		}
		// pending or processing
		return [
			{ label: 'Created', state: 'done' },
			{ label: 'Processing', state: 'active' },
			{ label: 'Complete', state: 'waiting' }
		];
	});
</script>

<div class="status-bar">
	{#each steps as step, i}
		<div class="step" class:done={step.state === 'done'} class:active={step.state === 'active'} class:error={step.state === 'error'}>
			<div class="dot">
				{#if step.state === 'done'}
					<svg viewBox="0 0 16 16" fill="currentColor"><path d="M12.207 4.793a1 1 0 010 1.414l-5 5a1 1 0 01-1.414 0l-2.5-2.5a1 1 0 011.414-1.414L6.5 9.086l4.293-4.293a1 1 0 011.414 0z"/></svg>
				{:else if step.state === 'error'}
					<svg viewBox="0 0 16 16" fill="currentColor"><path d="M4.646 4.646a.5.5 0 01.708 0L8 7.293l2.646-2.647a.5.5 0 01.708.708L8.707 8l2.647 2.646a.5.5 0 01-.708.708L8 8.707l-2.646 2.647a.5.5 0 01-.708-.708L7.293 8 4.646 5.354a.5.5 0 010-.708z"/></svg>
				{/if}
			</div>
			<span class="label">{step.label}</span>
		</div>
		{#if i < steps.length - 1}
			<div class="connector" class:done={step.state === 'done'} class:error={step.state === 'error'}></div>
		{/if}
	{/each}
</div>

<style>
	.status-bar {
		display: flex;
		align-items: center;
		gap: 0;
		padding: 1rem 0;
	}

	.step {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.375rem;
		flex-shrink: 0;
	}

	.dot {
		width: 24px;
		height: 24px;
		border-radius: 50%;
		background: #e5e7eb;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: all 0.3s;
	}

	.dot svg {
		width: 14px;
		height: 14px;
	}

	.done .dot {
		background: #4ecca3;
		color: white;
	}

	.active .dot {
		background: #3b82f6;
		color: white;
		animation: pulse 2s infinite;
	}

	.error .dot {
		background: #ef4444;
		color: white;
	}

	@keyframes pulse {
		0%, 100% { box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.4); }
		50% { box-shadow: 0 0 0 6px rgba(59, 130, 246, 0); }
	}

	.label {
		font-size: 0.7rem;
		color: #999;
		white-space: nowrap;
	}

	.done .label { color: #4ecca3; font-weight: 500; }
	.active .label { color: #3b82f6; font-weight: 500; }
	.error .label { color: #ef4444; font-weight: 500; }

	.connector {
		flex: 1;
		height: 2px;
		background: #e5e7eb;
		min-width: 20px;
		margin-bottom: 1.25rem;
		transition: background 0.3s;
	}

	.connector.done { background: #4ecca3; }
	.connector.error { background: #ef4444; }
</style>
