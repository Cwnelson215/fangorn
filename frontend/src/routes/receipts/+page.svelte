<script lang="ts">
	import { onMount } from 'svelte';
	import {
		getAccounts,
		getCategories,
		getReceipts,
		getTransactions,
		receiptImageUrl,
		uploadReceipt
	} from '$lib/api';
	import { downscale } from '$lib/image';
	import { formatCurrency, formatDateShort, formatSigned } from '$lib/format';
	import { startPolling } from '$lib/poll';
	import { describeUpload, isWorking, reasonText, type UploadNotice } from '$lib/receipts';
	import { RECEIPT_UPLOADED } from '$lib/capture.svelte';
	import type { Account, Category, Receipt, Transaction } from '$lib/types';
	import ReceiptReviewModal from '$lib/components/ReceiptReviewModal.svelte';
	import TransactionModal from '$lib/components/TransactionModal.svelte';

	// How long to keep checking on receipts still being read. Past this they are
	// still finished by the server; the page just stops watching.
	const POLL_MS = 3000;
	const POLL_FOR_MS = 2 * 60 * 1000;

	let receipts = $state<Receipt[]>([]);
	// Once a receipt is posted the transaction is what counts, so the Posted list
	// shows the transactions — as they stand now, edits included.
	let posted = $state<Transaction[]>([]);
	let accounts = $state<Account[]>([]);
	let categories = $state<Category[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let fileInput: HTMLInputElement;
	let stage = $state<'idle' | 'preparing' | 'uploading'>('idle');
	let notice = $state<UploadNotice | null>(null);

	let reviewing = $state<Receipt | null>(null);
	let reviewOpen = $state(false);

	let editing = $state<Transaction | null>(null);
	let editOpen = $state(false);

	let needsReview = $derived(receipts.filter((r) => r.status === 'needs_review'));
	let working = $derived(receipts.filter(isWorking));

	let accountName = $derived(new Map(accounts.map((a) => [a.id, a.name])));
	let categoryName = $derived(new Map(categories.map((c) => [c.id, c.name])));

	async function load() {
		try {
			[receipts, posted] = await Promise.all([
				getReceipts(),
				getTransactions({ receipt: true, limit: 20 })
			]);
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
		// A photo taken with the top bar's camera while this page is open.
		const refresh = () => load();
		window.addEventListener(RECEIPT_UPLOADED, refresh);
		return () => window.removeEventListener(RECEIPT_UPLOADED, refresh);
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
			notice = describeUpload(res, accountName, categoryName);
			await load();
			if (res.receipt.status === 'needs_review' && !res.duplicate) openReview(res.receipt);
		} catch (e) {
			notice = { text: e instanceof Error ? e.message : 'Upload failed', tone: 'error' };
		} finally {
			stage = 'idle';
		}
	}

	function openReview(r: Receipt) {
		reviewing = r;
		reviewOpen = true;
	}

	function openEdit(t: Transaction) {
		editing = t;
		editOpen = true;
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
				{#each posted as t (t.id)}
					<button class="item" onclick={() => openEdit(t)}>
						<img class="thumb" src={receiptImageUrl(t.receipt_id!)} alt="" loading="lazy" />
						<span class="body">
							<span class="title">{t.merchant ?? t.description}</span>
							<span class="why">
								{formatDateShort(t.date)}
								{#if t.account_name}· {t.account_name}{/if}
								{#if t.category_name}· {t.category_name}{/if}
							</span>
						</span>
						<span class="amount">{t.amount < 0 ? formatCurrency(-t.amount) : formatSigned(t.amount)}</span>
					</button>
				{/each}
			</div>
		{/if}
	</section>
</div>

<ReceiptReviewModal bind:open={reviewOpen} receipt={reviewing} {accounts} {categories} onchange={load} />

<TransactionModal {accounts} {categories} transaction={editing} bind:open={editOpen} onsaved={load} />

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
		color: var(--on-accent);
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
		background: var(--pos-soft);
		color: var(--pos);
	}

	.notice.warn {
		background: var(--warn-soft);
		color: var(--warn-text);
	}

	.notice.error {
		background: var(--neg-soft);
		color: var(--neg);
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

	button.item {
		cursor: pointer;
	}

	button.item:hover {
		background: var(--surface-2);
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
