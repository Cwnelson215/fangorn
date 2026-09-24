<script lang="ts">
	import { createTrade, deleteTrade, getSecurity, updateTrade } from '$lib/api';
	import type { SecurityMatch, Trade, TradeInput, TradeSide } from '$lib/types';
	import { TRADE_SIDE_LABELS } from '$lib/types';
	import { formatCurrency, formatPrice, today } from '$lib/format';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';
	import SymbolSearch from './SymbolSearch.svelte';

	let {
		accountId,
		trade = null,
		open = $bindable(false),
		onsaved
	}: {
		accountId: number;
		/** The trade being edited, or null to log a new one. */
		trade?: Trade | null;
		open?: boolean;
		onsaved: () => void;
	} = $props();

	let side = $state<TradeSide>('buy');
	let symbol = $state('');
	let securityName = $state<string | null>(null);
	let date = $state(today());
	let shares = $state<string | number | null>('');
	let price = $state<string | number | null>('');
	let fees = $state<string | number | null>('');
	// Blank means "shares × price ± fees"; filled in means the real figure from
	// the confirmation, which for a "$500 of FZROX" order is what actually moved.
	let total = $state<string | number | null>('');
	let notes = $state('');

	let quoteNote = $state<string | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	// Reset the form each time the modal opens, from the trade if editing.
	$effect(() => {
		if (!open) return;
		formError = null;
		quoteNote = null;
		if (trade) {
			side = trade.side;
			symbol = trade.symbol;
			securityName = trade.security_name;
			date = trade.trade_date;
			shares = String(trade.shares);
			price = String(trade.price);
			fees = trade.fees ? String(trade.fees) : '';
			total = trade.amount !== computeTotal(trade.side, trade.shares, trade.price, trade.fees)
				? String(trade.amount)
				: '';
			notes = trade.notes ?? '';
		} else {
			side = 'buy';
			symbol = '';
			securityName = null;
			date = today();
			shares = '';
			price = '';
			fees = '';
			total = '';
			notes = '';
		}
	});

	let movesCash = $derived(side === 'buy' || side === 'sell');

	// bind:value on a number input yields a number, or null once cleared — not
	// the '' these fields start as — so every read goes through these two.
	function isBlank(v: unknown): boolean {
		return v === '' || v === null || v === undefined;
	}
	function num(v: unknown): number {
		return parseFloat(String(v ?? '')) || 0;
	}

	function computeTotal(s: TradeSide, n: number, p: number, f: number): number {
		const gross = n * p;
		const withFees = s === 'buy' ? gross + f : s === 'sell' ? gross - f : gross;
		return Math.round(withFees * 100) / 100;
	}

	let computedTotal = $derived(
		computeTotal(side, num(shares), num(price), movesCash ? num(fees) : 0)
	);
	let effectiveTotal = $derived(isBlank(total) ? computedTotal : num(total));

	let cashLine = $derived.by(() => {
		if (!movesCash) return "Doesn't change this account's cash";
		if (effectiveTotal <= 0) return '';
		return side === 'buy'
			? `Takes ${formatCurrency(effectiveTotal)} from this account's cash`
			: `Adds ${formatCurrency(effectiveTotal)} to this account's cash`;
	});

	async function onSelect(match: SecurityMatch) {
		securityName = match.name;
		await prefillPrice(match.symbol);
	}

	// Prefill the current price when logging something today. For a back-dated
	// trade today's price would be wrong, so it is shown but not filled in.
	async function prefillPrice(sym: string) {
		quoteNote = null;
		try {
			const sec = await getSecurity(sym);
			securityName = sec.name ?? securityName;
			if (date === today() && isBlank(price)) {
				price = String(sec.price);
				quoteNote = 'Current price — change it to what you actually paid.';
			} else {
				quoteNote = `Currently ${formatPrice(sec.price)}`;
			}
		} catch {
			// Unknown or unreachable: the server reports it properly on save.
		}
	}

	function buildInput(): TradeInput {
		return {
			symbol: symbol.trim().toUpperCase(),
			side,
			trade_date: date,
			shares: num(shares),
			price: num(price),
			fees: movesCash ? num(fees) : 0,
			amount: isBlank(total) ? null : num(total),
			notes: notes.trim() || null
		};
	}

	async function handleSubmit(event: Event) {
		event.preventDefault();
		saving = true;
		formError = null;
		try {
			if (trade) {
				await updateTrade(trade.id, buildInput());
			} else {
				await createTrade(accountId, buildInput());
			}
			open = false;
			onsaved();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the trade';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!trade) return;
		deleting = true;
		formError = null;
		try {
			await deleteTrade(trade.id);
			open = false;
			onsaved();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete the trade';
		} finally {
			deleting = false;
		}
	}

	const SIDES: TradeSide[] = ['buy', 'sell', 'reinvest', 'opening'];
	const SIDE_HINTS: Record<TradeSide, string> = {
		buy: 'Bought with cash in this account.',
		sell: 'Sold, with the proceeds kept as cash in this account.',
		reinvest: 'A dividend that bought more shares instead of paying out cash.',
		opening: 'Shares you held before you started tracking this account.'
	};
</script>

<Modal bind:open title={trade ? 'Edit Trade' : 'Log Trade'}>
	<form onsubmit={handleSubmit}>
		<div class="toggle" role="radiogroup" aria-label="Trade type">
			{#each SIDES as s (s)}
				<button
					type="button"
					role="radio"
					aria-checked={side === s}
					class:active={side === s}
					onclick={() => (side = s)}
				>
					{TRADE_SIDE_LABELS[s]}
				</button>
			{/each}
		</div>
		<p class="side-hint">{SIDE_HINTS[side]}</p>

		<Field label="Symbol" id="symbol" hint={securityName ?? undefined}>
			<SymbolSearch id="symbol" bind:value={symbol} disabled={saving} onselect={onSelect} />
		</Field>

		<div class="form-row">
			<Field label="Shares" id="shares">
				<input
					id="shares"
					type="number"
					inputmode="decimal"
					step="any"
					min="0"
					placeholder="0"
					bind:value={shares}
					disabled={saving}
					required
				/>
			</Field>
			<Field
				label={side === 'opening' ? 'Average price paid' : 'Price per share'}
				id="price"
				hint={quoteNote ?? undefined}
			>
				<input
					id="price"
					type="number"
					inputmode="decimal"
					step="any"
					min="0"
					placeholder="0.00"
					bind:value={price}
					disabled={saving}
					onfocus={() => symbol.trim() && isBlank(price) && prefillPrice(symbol.trim().toUpperCase())}
					required
				/>
			</Field>
		</div>

		<div class="form-row">
			<Field label="Date" id="tradeDate">
				<input id="tradeDate" type="date" bind:value={date} disabled={saving} required />
			</Field>
			{#if movesCash}
				<Field label="Fees" id="fees">
					<input
						id="fees"
						type="number"
						inputmode="decimal"
						step="0.01"
						min="0"
						placeholder="0.00"
						bind:value={fees}
						disabled={saving}
					/>
				</Field>
			{/if}
		</div>

		<Field
			label={movesCash ? (side === 'buy' ? 'Total paid' : 'Total received') : 'Cost basis'}
			id="total"
			hint="Leave blank to use shares × price{movesCash ? ' ± fees' : ''}. Enter the real figure if your statement differs."
		>
			<input
				id="total"
				type="number"
				inputmode="decimal"
				step="0.01"
				min="0"
				placeholder={computedTotal ? computedTotal.toFixed(2) : '0.00'}
				bind:value={total}
				disabled={saving}
			/>
		</Field>

		{#if cashLine}
			<p class="cash-line">{cashLine}</p>
		{/if}

		<Field label="Notes" id="tradeNotes">
			<textarea id="tradeNotes" bind:value={notes} disabled={saving}></textarea>
		</Field>

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			{#if trade}
				<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
					{deleting ? 'Deleting…' : 'Delete'}
				</Button>
			{/if}
			<span class="spacer"></span>
			<Button variant="secondary" onclick={() => (open = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !symbol.trim() || !num(shares) || isBlank(price)}>
				{saving ? 'Saving…' : trade ? 'Save Changes' : 'Log Trade'}
			</Button>
		</div>
	</form>
</Modal>

<style>
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
	}

	.spacer {
		flex: 1;
	}

	.toggle {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		background: var(--bg);
		border-radius: var(--radius-sm);
		padding: 0.25rem;
		gap: 0.25rem;
	}

	.toggle button {
		padding: 0.5rem 0.25rem;
		border: none;
		background: none;
		border-radius: 6px;
		font: inherit;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--muted);
		cursor: pointer;
	}

	.toggle button.active {
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow);
	}

	.side-hint {
		margin: -0.5rem 0 0;
		font-size: 0.8125rem;
		color: var(--muted);
	}

	.cash-line {
		margin: -0.25rem 0 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ink);
	}

	@media (max-width: 480px) {
		.toggle {
			grid-template-columns: repeat(2, 1fr);
		}
	}
</style>
