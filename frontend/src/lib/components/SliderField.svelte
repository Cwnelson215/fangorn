<script lang="ts">
	// A projection assumption: a slider to drag and, beside it, the exact number
	// to type. The slider's range is a comfortable default, not a limit — a
	// typed value past its end is kept, and the slider just sits at its end.
	let {
		label,
		id,
		value = $bindable(),
		min,
		max,
		step,
		prefix = '',
		suffix = '',
		hint
	}: {
		label: string;
		id: string;
		value: number;
		min: number;
		max: number;
		step: number;
		prefix?: string;
		suffix?: string;
		hint?: string;
	} = $props();

	function typed(event: Event) {
		const n = Number((event.currentTarget as HTMLInputElement).value);
		if (Number.isFinite(n) && n >= min) value = n;
	}
</script>

<div class="slider-field">
	<div class="top">
		<label for={id}>{label}</label>
		<span class="exact">
			{#if prefix}<span class="affix">{prefix}</span>{/if}
			<input
				type="number"
				inputmode="decimal"
				aria-label={label}
				{min}
				{step}
				value={value}
				onchange={typed}
			/>
			{#if suffix}<span class="affix">{suffix}</span>{/if}
		</span>
	</div>
	<input {id} type="range" {min} {max} {step} bind:value />
	{#if hint}<span class="hint">{hint}</span>{/if}
</div>

<style>
	.slider-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0;
	}

	.top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	label {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--muted);
	}

	.exact {
		display: inline-flex;
		align-items: center;
		gap: 0.125rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.affix {
		color: var(--muted);
		font-weight: 600;
	}

	input[type='number'] {
		width: 5.5rem;
		padding: 0.25rem 0.375rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		font: inherit;
		font-weight: 700;
		text-align: right;
		background: var(--surface);
		color: var(--ink);
	}

	input[type='number']:focus {
		outline: none;
		border-color: var(--accent);
	}

	input[type='range'] {
		width: 100%;
		accent-color: var(--ink);
		/* A comfortable thumb to grab on a phone. */
		min-height: 28px;
	}

	.hint {
		font-size: 0.75rem;
		color: var(--muted-light);
	}

	@media (pointer: coarse) {
		input[type='number'] {
			font-size: 1rem;
			min-height: 36px;
		}
	}
</style>
