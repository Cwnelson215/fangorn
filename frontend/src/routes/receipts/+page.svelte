<script lang="ts">
	import { onMount } from 'svelte';
	import { getAccounts, getCategories, getReceipts, receiptImageUrl, uploadReceipt } from '$lib/api';
	import { downscale } from '$lib/image';
	import { formatCurrency, formatDateShort } from '$lib/format';
	import { startPolling } from '$lib/poll';
	import { isWorking, reasonText } from '$lib/receipts';
	import type { Account, Category, Receipt } from '$lib/types';
	import ReceiptReviewModal from '$lib/components/ReceiptReviewModal.svelte';

	// How long to keep checking on receipts still being read. Past this they are
	// still finished by the server; the page just stops watching.
	const POLL_MS = 3000;
	const POLL_FOR_MS = 2 * 60 * 1000;

	let receipts = $state<Receipt[]>([]);
	let accounts = $state<Account[]>([]);
	let categories = $state<Category[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let fileInput: HTMLInputElement;
	let stage = $state<'idle' | 'preparing' | 'uploading'>('idle');
	let notice = $state<{ text: string; tone: 'ok' | 'warn' | 'error' } | null>(null);

	let reviewing = $state<Receipt | null>(null);
	let reviewOpen = $state(false);

	let needsReview = $derived(receipts.filter((r) => r.status === 'needs_review'));
	let working = $derived(receipts.filter(isWorking));
	let posted = $derived(receipts.filter((r) => r.status === 'posted').slice(0, 20));

	let accountName = $derived(new Map(accounts.map((a) => [a.id, a.name])));
	let categoryName = $derived(new Map(categories.map((c) => [c.id, c.name])));

	async function load() {
		try {
			receipts = await getReceipts();
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load receipts';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		Promise.all([getAccounts(), getCategories()])
			.then(([a, c]) => {
				accounts = a;
				categories = c;
			})
			.catch(() => {});
		load();
	});

	// Poll while anything is still being read, for a while. Keyed on a boolean
	// so each refresh doesn't tear the poller down and start it again.
	let hasWorking = $derived(working.length > 0);
	let pollStarted = 0;
	$effect(() => {
		if (!hasWorking) {
			pollStarted = 0;
			return;
		}
		if (!pollStarted) pollStarted = Date.now();
		const stop = startPolling(async () => {
			if (Date.now() - pollStarted > POLL_FOR_MS) {
				stop();
				return;
			}
			await load();
		}, POLL_MS);
		return stop;
	});

	async function onFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = ''; // so choosing the same file again still fires
		if (!file) return;

		notice = null;
		try {
			stage = 'preparing';
			const image = await downscale(file);
			stage = 'uploading';
			const res = await uploadReceipt(image);
			notice = describe(res.receipt, res.duplicate, res.enabled);
			await load();
			if (res.receipt.status === 'needs_review' && !res.duplicate) openReview(res.receipt);
		} catch (e) {
			notice = { text: e instanceof Error ? e.message : 'Upload failed', tone: 'error' };
		} finally {
			stage = 'idle';
		}
	}

	function describe(r: Receipt, duplicate: boolean, enabled: boolean): NonNullable<typeof notice> {
		if (duplicate) return { text: 'That photo was already uploaded.', tone: 'warn' };
		if (r.status === 'posted') {
			const parts = [
				formatCurrency(r.total ?? 0),
				r.merchant ? `at ${r.merchant}` : null,
				r.account_id ? `on ${accountName.get(r.account_id) ?? 'your account'}` : null,
				r.category_id ? `under ${categoryName.get(r.category_id) ?? 'its category'}` : null
			];
			return { text: `Posted ${parts.filter(Boolean).join(' ')}.`, tone: 'ok' };
		}
		if (r.status === 'needs_review') {
			return {
				text: enabled ? 'Read it, but it needs a look before it goes in.' : 'Saved. Enter the details below.',
				tone: 'warn'
			};
		}
		return { text: "Still reading it. It'll post on its own, or show up below if it needs you.", tone: 'ok' };
	}

	function openReview(r: Receipt) {
		reviewing = r;
		reviewOpen = true;
	}
</script>

<div class="page">
	<div class="page-header">
		<h1>Receipts</h1>
	</div>

	<div class="card capture">
		<input
			bind:this={fileInput}
			class="hidden"
			type="file"
			accept="image/*"
			capture="environment"
			onchange={onFile}
		/>
		<button class="shoot" onclick={() => fileInput.click()} disabled={stage !== 'idle'}>
			{#if stage === 'preparing'}
				Preparing photo…
			{:else if stage === 'uploading'}
				Reading receipt…
			{:else}
				Scan a receipt
			{/if}
		</button>
		<p class="muted small">
			Take a photo, flat and well lit. Expenses post on their own when the card's last 4 digits match
			an account and the category matches one of yours; anything else waits below for you.
		</p>
		{#if notice}
			<p class="notice {notice.tone}">{notice.text}</p>
		{/if}
	</div>

	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	{#if needsReview.length > 0}
		<section>
			<h2>Needs a look <span class="count">{needsReview.length}</span></h2>
			<div class="card list">
				{#each needsReview as r (r.id)}
					<button class="item" onclick={() => openReview(r)}>
						<img class="thumb" src={receiptImageUrl(r.id)} alt="" loading="lazy" />
						<span class="body">
							<span class="title">{r.merchant ?? 'Receipt'}</span>
							<span class="why">{reasonText(r.review_reasons[0] ?? '', r)}</span>
						</span>
						<span class="amount">{r.total != null ? formatCurrency(r.total) : '—'}</span>
					</button>
				{/each}
			</div>
		</section>
	{/if}

	{#if working.length > 0}
		<section>
			<h2>Reading <span class="count">{working.length}</span></h2>
			<div class="card list">
				{#each working as r (r.id)}
					<div class="item">
						<img class="thumb" src={receiptImageUrl(r.id)} alt="" loading="lazy" />
						<span class="body">
							<span class="title">Uploaded {formatDateShort(r.created_at.slice(0, 10))}</span>
							<span class="why">{r.extract_error ? 'Retrying shortly' : 'Being read…'}</span>
						</span>
					</div>
				{/each}
			</div>
		</section>
	{/if}

	<section>
		<h2>Posted</h2>
		{#if loading}
			<p class="muted">Loading…</p>
		{:else if posted.length === 0}
			<div class="card empty"><p class="muted">Nothing posted from a receipt yet.</p></div>
		{:else}
			<div class="card list">
				{#each posted as r (r.id)}
					<a class="item" href={receiptImageUrl(r.id)} target="_blank" rel="noopener">
						<img class="thumb" src={receiptImageUrl(r.id)} alt="" loading="lazy" />
						<span class="body">
							<span class="title">{r.merchant ?? 'Receipt'}</span>
							<span class="why">
								{r.purchased_on ? formatDateShort(r.purchased_on) : ''}
								{#if r.account_id}· {accountName.get(r.account_id) ?? ''}{/if}
								{#if r.category_id}· {categoryName.get(r.category_id) ?? ''}{/if}
							</span>
						</span>
						<span class="amount">{r.total != null ? formatCurrency(r.total) : ''}</span>
					</a>
				{/each}
			</div>
		{/if}
	</section>
</div>

<ReceiptReviewModal bind:open={reviewOpen} receipt={reviewing} {accounts} {categories} onchange={load} />

<style>
	.hidden {
		display: none;
	}

	.capture {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
		text-align: center;
		margin-bottom: 1.5rem;
	}

	.shoot {
		width: 100%;
		max-width: 360px;
		padding: 1.25rem;
		font: inherit;
		font-size: 1.125rem;
		font-weight: 700;
		color: #fff;
		background: var(--accent);
		border: none;
		border-radius: var(--radius);
		cursor: pointer;
	}

	.shoot:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.shoot:disabled {
		opacity: 0.7;
		cursor: progress;
	}

	.small {
		font-size: 0.85rem;
		max-width: 480px;
		margin: 0;
	}

	.notice {
		margin: 0;
		padding: 0.6rem 1rem;
		border-radius: var(--radius-sm);
		font-size: 0.9rem;
	}

	.notice.ok {
		background: #e8f5e9;
		color: #2e7d32;
	}

	.notice.warn {
		background: #fff8e6;
		color: #8a5a00;
	}

	.notice.error {
		background: #fdecea;
		color: #b3261e;
	}

	section {
		margin-bottom: 1.5rem;
	}

	h2 {
		font-size: 1rem;
		margin: 0 0 0.5rem;
	}

	.count {
		font-size: 0.8rem;
		color: var(--muted);
		font-weight: 500;
	}

	.list {
		padding: 0;
		overflow: hidden;
	}

	.item {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
		padding: 0.75rem 1rem;
		border: none;
		border-bottom: 1px solid var(--divider);
		background: none;
		font: inherit;
		color: inherit;
		text-align: left;
		text-decoration: none;
	}

	.item:last-child {
		border-bottom: none;
	}

	button.item,
	a.item {
		cursor: pointer;
	}

	button.item:hover,
	a.item:hover {
		background: #fafafa;
	}

	.thumb {
		width: 44px;
		height: 56px;
		object-fit: cover;
		border-radius: 4px;
		background: var(--bg);
		flex-shrink: 0;
	}

	.body {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}

	.title {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.why {
		font-size: 0.8rem;
		color: var(--muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.amount {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
</style>
