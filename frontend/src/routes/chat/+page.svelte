<script lang="ts">
	// Ask: a conversation with Claude about the household's money. It reads the
	// ledger through the server's tools and can't change anything.
	//
	// /chat is a new conversation (with recent ones listed under it until the
	// first question); /chat?id=N is a saved one. A new chat is only created when
	// its first question is sent, and the URL then follows it.
	import { onDestroy, tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { askChat, createChat, deleteChat, getChat, getChats } from '$lib/api';
	import type { Chat, ChatMessage } from '$lib/types';
	import { renderMarkdown } from '$lib/markdown';
	import Button from '$lib/components/Button.svelte';

	const SUGGESTIONS = [
		'Where did our money go this month?',
		'How much did we spend on groceries last month?',
		'Are we on track with this month’s budget?',
		'How are our savings goals doing?',
		'What subscriptions and bills do we pay every month?',
		'How has our net worth changed this year?'
	];

	let enabled = $state(true);
	let chats = $state<Chat[]>([]);
	let chatId = $state<number | null>(null);
	let title = $state('');
	let messages = $state<ChatMessage[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// The answer being written, before it is saved.
	let busy = $state(false);
	let draft = $state('');
	let liveTools = $state<string[]>([]);
	let input = $state('');
	let confirmingDelete = $state<number | null>(null);

	let abort: AbortController | null = null;
	let thread: HTMLElement | undefined = $state();
	let box: HTMLTextAreaElement | undefined = $state();

	let urlId = $derived(Number(page.url.searchParams.get('id')) || null);

	// Load whatever the URL names — unless it is the chat already on screen,
	// which is the case right after a new chat's first question moves the URL.
	$effect(() => {
		const id = urlId;
		untrack(() => {
			if (id !== null && id === chatId) return;
			load(id);
		});
	});

	async function load(id: number | null) {
		abort?.abort();
		busy = false;
		draft = '';
		liveTools = [];
		error = null;
		loading = true;
		try {
			const list = await getChats();
			enabled = list.enabled;
			chats = list.chats;
			if (id === null) {
				chatId = null;
				title = '';
				messages = [];
			} else {
				const res = await getChat(id);
				chatId = res.chat.id;
				title = res.chat.title;
				messages = res.messages;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the conversation';
		} finally {
			loading = false;
		}
		await scrollDown();
	}

	onDestroy(() => abort?.abort());

	async function scrollDown() {
		await tick();
		thread?.lastElementChild?.scrollIntoView({ block: 'end' });
	}

	async function ask(text: string) {
		text = text.trim();
		if (!text || busy) return;
		error = null;
		busy = true;
		draft = '';
		liveTools = [];
		input = '';
		messages = [...messages, { role: 'user', text }];
		await scrollDown();

		abort = new AbortController();
		// Text after a lookup starts a new paragraph, as it does once saved.
		let afterTool = false;
		try {
			if (chatId === null) {
				const chat = await createChat();
				chatId = chat.id;
				goto(`/chat?id=${chat.id}`, { replaceState: true, noScroll: true, keepFocus: true });
			}
			const chat = await askChat(
				chatId,
				text,
				{
					onText: (t) => {
						if (afterTool && draft) draft += '\n\n';
						afterTool = false;
						draft += t;
						scrollDown();
					},
					onTool: (label) => {
						afterTool = true;
						if (!liveTools.includes(label)) liveTools = [...liveTools, label];
					}
				},
				abort.signal
			);
			messages = [...messages, { role: 'assistant', text: draft, tools: liveTools }];
			title = chat.title;
		} catch (e) {
			if (abort?.signal.aborted) return;
			// Nothing was saved: take the question back so it can be asked again.
			messages = messages.slice(0, -1);
			input = text;
			error = e instanceof Error ? e.message : 'Something went wrong';
		} finally {
			busy = false;
			draft = '';
			liveTools = [];
			abort = null;
		}
		await scrollDown();
		box?.focus();
	}

	function onKeydown(event: KeyboardEvent) {
		// Enter sends; Shift+Enter is a new line. A composing IME keeps Enter.
		if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
			event.preventDefault();
			ask(input);
		}
	}

	async function remove(id: number) {
		if (confirmingDelete !== id) {
			confirmingDelete = id;
			return;
		}
		confirmingDelete = null;
		try {
			await deleteChat(id);
			chats = chats.filter((c) => c.id !== id);
			if (id === chatId) goto('/chat');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not delete that conversation';
		}
	}

	function when(iso: string): string {
		const d = new Date(iso);
		const today = new Date();
		if (d.toDateString() === today.toDateString()) {
			return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
		}
		return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}

	let empty = $derived(messages.length === 0 && !busy);
</script>

<div class="page chat-page">
	<div class="page-header">
		<h1>{chatId && title ? title : 'Ask'}</h1>
		{#if chatId !== null}
			<div class="header-actions">
				<Button variant="ghost" size="sm" onclick={() => remove(chatId!)}>
					{confirmingDelete === chatId ? 'Tap again to delete' : 'Delete'}
				</Button>
				<Button variant="secondary" size="sm" onclick={() => goto('/chat')}>New chat</Button>
			</div>
		{/if}
	</div>

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if !enabled}
		<div class="card empty">
			The assistant isn't set up on this server. It needs <code>ANTHROPIC_API_KEY</code>.
		</div>
	{:else}
		{#if empty}
			<section class="intro">
				<p class="muted">
					Ask about your accounts, spending, budgets, goals, bills or investments. Claude looks things up
					in your ledger to answer — it can read everything here but can't change anything.
				</p>
				<div class="suggestions">
					{#each SUGGESTIONS as s (s)}
						<button class="chip" onclick={() => ask(s)}>{s}</button>
					{/each}
				</div>
			</section>

			{#if chatId === null && chats.length > 0}
				<section class="card recent">
					<h2>Recent</h2>
					<ul>
						{#each chats as c (c.id)}
							<li>
								<a href="/chat?id={c.id}">
									<span class="recent-title">{c.title || 'Untitled'}</span>
									<span class="muted small">{when(c.updated_at)}</span>
								</a>
								<button
									class="recent-delete"
									aria-label={confirmingDelete === c.id ? 'Confirm delete' : `Delete ${c.title}`}
									onclick={() => remove(c.id)}
								>
									{confirmingDelete === c.id ? 'Delete?' : '×'}
								</button>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		{/if}

		<div class="thread" bind:this={thread} aria-live="polite">
			{#each messages as m, i (i)}
				{#if m.role === 'user'}
					<div class="bubble user">{m.text}</div>
				{:else}
					<div class="bubble assistant">
						{#if m.tools?.length}
							<p class="looked-up">Looked at: {m.tools.join(', ').toLowerCase()}</p>
						{/if}
						<!-- renderMarkdown escapes all text before adding markup. -->
						<div class="md">{@html renderMarkdown(m.text)}</div>
					</div>
				{/if}
			{/each}

			{#if busy}
				<div class="bubble assistant live">
					{#if liveTools.length}
						<p class="looked-up">{liveTools[liveTools.length - 1]}…</p>
					{/if}
					{#if draft}
						<div class="md">{@html renderMarkdown(draft)}</div>
					{:else if !liveTools.length}
						<p class="looked-up">Thinking…</p>
					{/if}
				</div>
			{/if}
		</div>

		{#if error}
			<p class="error-text">{error}</p>
		{/if}

		<form
			class="composer"
			onsubmit={(e) => {
				e.preventDefault();
				ask(input);
			}}
		>
			<textarea
				bind:this={box}
				bind:value={input}
				onkeydown={onKeydown}
				rows="1"
				maxlength="4000"
				placeholder="Ask about your money…"
				aria-label="Question"
				enterkeyhint="send"
			></textarea>
			<Button type="submit" disabled={busy || !input.trim()}>{busy ? '…' : 'Ask'}</Button>
		</form>
	{/if}
</div>

<style>
	.chat-page {
		gap: 1rem;
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
	}

	.page-header h1 {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
		flex: 1;
	}

	.header-actions {
		display: flex;
		gap: 0.5rem;
	}

	.intro {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.suggestions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.chip {
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: 999px;
		color: var(--ink);
		font: inherit;
		font-size: 0.875rem;
		padding: 0.4rem 0.85rem;
		cursor: pointer;
		text-align: left;
	}

	.chip:hover {
		border-color: var(--muted);
	}

	.recent h2 {
		font-size: 1rem;
		margin-bottom: 0.5rem;
	}

	.recent ul {
		list-style: none;
	}

	.recent li {
		display: flex;
		align-items: center;
		border-top: 1px solid var(--line);
	}

	.recent li:first-child {
		border-top: none;
	}

	.recent a {
		flex: 1;
		min-width: 0;
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.6rem 0;
		color: var(--ink);
		text-decoration: none;
	}

	.recent-title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.small {
		font-size: 0.8125rem;
		white-space: nowrap;
	}

	.recent-delete {
		background: none;
		border: none;
		color: var(--muted);
		font: inherit;
		cursor: pointer;
		min-width: 44px;
		min-height: 36px;
	}

	.recent-delete:hover {
		color: var(--neg);
	}

	.thread {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.bubble {
		border-radius: var(--radius);
		padding: 0.75rem 1rem;
		max-width: 100%;
		overflow-wrap: anywhere;
	}

	.bubble.user {
		align-self: flex-end;
		max-width: 85%;
		background: var(--surface-2);
		border: 1px solid var(--border);
		white-space: pre-wrap;
	}

	.bubble.assistant {
		align-self: stretch;
		background: var(--surface);
		border: 1px solid var(--divider);
	}

	.looked-up {
		color: var(--muted-light);
		font-size: 0.8125rem;
		margin-bottom: 0.35rem;
	}

	.live .looked-up:only-child {
		margin-bottom: 0;
	}

	.md :global(p) {
		margin: 0 0 0.6rem;
	}

	.md :global(p:last-child),
	.md :global(ul:last-child),
	.md :global(ol:last-child),
	.md :global(.md-table:last-child) {
		margin-bottom: 0;
	}

	.md :global(.md-heading) {
		font-weight: 700;
	}

	.md :global(ul),
	.md :global(ol) {
		margin: 0 0 0.6rem 1.25rem;
	}

	.md :global(li) {
		margin: 0.15rem 0;
	}

	.md :global(code) {
		background: var(--surface-2);
		border-radius: 4px;
		padding: 0 0.3rem;
		font-size: 0.9em;
	}

	.md :global(.md-table) {
		overflow-x: auto;
		margin: 0 0 0.6rem;
	}

	.md :global(table) {
		border-collapse: collapse;
		font-size: 0.875rem;
		font-variant-numeric: tabular-nums;
	}

	.md :global(th),
	.md :global(td) {
		text-align: left;
		padding: 0.3rem 0.75rem 0.3rem 0;
		border-bottom: 1px solid var(--line);
		white-space: nowrap;
	}

	.md :global(th) {
		color: var(--muted);
		font-weight: 600;
	}

	.md :global(.num) {
		text-align: right;
	}

	.composer {
		position: sticky;
		bottom: 0;
		display: flex;
		gap: 0.5rem;
		align-items: flex-end;
		padding: 0.75rem 0;
		background: var(--bg);
	}

	.composer textarea {
		flex: 1;
		resize: none;
		field-sizing: content;
		min-height: 44px;
		max-height: 10rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		font: inherit;
		/* 16px keeps iOS from zooming in on focus. */
		font-size: 1rem;
	}

	/* The phone tab bar is fixed over the bottom of the page. */
	@media (max-width: 899px) {
		.composer {
			bottom: calc(64px + env(safe-area-inset-bottom));
		}
	}
</style>
