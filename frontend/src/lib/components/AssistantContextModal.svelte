<script lang="ts">
	// What Ask knows beyond the ledger: the family's own instructions and files,
	// the way a claude.ai Project holds them. Every new chat starts with what's
	// here; a chat already going keeps what it started with.
	import { untrack } from 'svelte';
	import {
		getAssistantContext,
		removeAssistantFile,
		saveAssistantInstructions,
		uploadAssistantFile
	} from '$lib/api';
	import type { AssistantFile } from '$lib/types';
	import Modal from './Modal.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';

	let {
		open = $bindable(false),
		onchange
	}: {
		open?: boolean;
		/** Called after anything is saved, with the current instructions and files. */
		onchange?: (instructions: string, files: AssistantFile[]) => void;
	} = $props();

	const MAX_TOTAL = 20 * 1024 * 1024;
	const ACCEPT = '.pdf,.txt,.md,.markdown,.csv,.tsv,.json,application/pdf,text/plain,text/csv,text/markdown';

	let loading = $state(true);
	let error = $state<string | null>(null);
	let saved = $state('');
	let instructions = $state('');
	let files = $state<AssistantFile[]>([]);
	let saving = $state(false);
	let uploading = $state<string | null>(null);
	let confirmingRemove = $state<number | null>(null);
	let picker: HTMLInputElement | undefined = $state();

	$effect(() => {
		if (!open) return;
		untrack(load);
	});

	async function load() {
		loading = true;
		error = null;
		confirmingRemove = null;
		try {
			const c = await getAssistantContext();
			saved = instructions = c.instructions;
			files = c.files;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load';
		} finally {
			loading = false;
		}
	}

	let dirty = $derived(instructions.trim() !== saved);
	let total = $derived(files.reduce((sum, f) => sum + f.size, 0));

	async function save() {
		saving = true;
		error = null;
		try {
			const res = await saveAssistantInstructions(instructions);
			saved = instructions = res.instructions;
			onchange?.(saved, files);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save the instructions';
		} finally {
			saving = false;
		}
	}

	async function upload(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const picked = Array.from(input.files ?? []);
		input.value = '';
		error = null;
		for (const file of picked) {
			uploading = file.name;
			try {
				files = [...files, await uploadAssistantFile(file)];
				onchange?.(saved, files);
			} catch (e) {
				error = `${file.name}: ${e instanceof Error ? e.message : 'could not upload'}`;
				break;
			}
		}
		uploading = null;
	}

	async function remove(id: number) {
		if (confirmingRemove !== id) {
			confirmingRemove = id;
			return;
		}
		confirmingRemove = null;
		error = null;
		try {
			await removeAssistantFile(id);
			files = files.filter((f) => f.id !== id);
			onchange?.(saved, files);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not remove that file';
		}
	}

	function size(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
		return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
	}
</script>

<Modal bind:open title="What Claude knows">
	<div class="body">
		<p class="muted">
			Like a Claude Project: Claude reads these with every question, alongside your ledger. New chats
			use what's here; a chat already going keeps what it started with.
		</p>

		{#if loading}
			<p class="muted">Loading…</p>
		{:else}
			<Field
				label="Instructions"
				id="assistantInstructions"
				hint="Who you are, what you're planning, how you want answers. Paste your Project's instructions here."
			>
				<textarea
					id="assistantInstructions"
					rows="10"
					bind:value={instructions}
					disabled={saving}
					placeholder="We're a family of four saving for a house by 2028…"
				></textarea>
			</Field>
			<div class="row-end">
				<Button size="sm" onclick={save} disabled={saving || !dirty}>
					{saving ? 'Saving…' : dirty ? 'Save instructions' : 'Saved'}
				</Button>
			</div>

			<section class="files">
				<div class="files-head">
					<h3>Files</h3>
					<span class="muted small">{size(total)} of {size(MAX_TOTAL)}</span>
				</div>
				{#if files.length === 0}
					<p class="muted small">
						None yet. PDFs and plain text (.txt, .md, .csv) — save Word documents and spreadsheets as
						PDF or CSV first.
					</p>
				{:else}
					<ul>
						{#each files as f (f.id)}
							<li>
								<span class="name">{f.name}</span>
								<span class="muted small">{size(f.size)}</span>
								<button
									class="remove"
									aria-label={confirmingRemove === f.id ? 'Confirm remove' : `Remove ${f.name}`}
									onclick={() => remove(f.id)}
								>
									{confirmingRemove === f.id ? 'Remove?' : '×'}
								</button>
							</li>
						{/each}
					</ul>
				{/if}
				<input
					bind:this={picker}
					class="picker"
					type="file"
					accept={ACCEPT}
					multiple
					onchange={upload}
				/>
				<div>
					<Button
						variant="secondary"
						size="sm"
						onclick={() => picker?.click()}
						disabled={uploading !== null}
					>
						{uploading ? `Uploading ${uploading}…` : 'Add files'}
					</Button>
				</div>
				<p class="muted small">
					Everything here goes with every question, so long files make each answer slower and cost more.
				</p>
			</section>
		{/if}

		{#if error}
			<p class="error-text">{error}</p>
		{/if}
	</div>
</Modal>

<style>
	.body {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	textarea {
		min-height: 10rem;
		resize: vertical;
	}

	.row-end {
		display: flex;
		justify-content: flex-end;
		margin-top: -0.5rem;
	}

	.files {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.files-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}

	h3 {
		font-size: 0.95rem;
	}

	ul {
		list-style: none;
	}

	li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		border-top: 1px solid var(--line);
		padding: 0.25rem 0;
	}

	li:first-child {
		border-top: none;
	}

	.name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.small {
		font-size: 0.8125rem;
	}

	.remove {
		background: none;
		border: none;
		color: var(--muted);
		font: inherit;
		cursor: pointer;
		min-width: 44px;
		min-height: 36px;
	}

	.remove:hover {
		color: var(--neg);
	}

	.picker {
		display: none;
	}
</style>
