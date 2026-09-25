<script lang="ts">
	import { onMount } from 'svelte';
	import {
		createCategory,
		deleteCategory,
		getAccounts,
		getCategories,
		unarchiveCategory,
		updateCategory
	} from '$lib/api';
	import type { Account, Category, CategoryKind } from '$lib/types';
	import AccountOptions from '$lib/components/AccountOptions.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';

	const DEFAULT_COLOR = '#3987e5';

	let categories: Category[] = $state([]);
	let accounts: Account[] = $state([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let notice = $state<string | null>(null);
	let showArchived = $state(false);

	// Form state
	let modalOpen = $state(false);
	let editing = $state<Category | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let formError = $state<string | null>(null);

	let name = $state('');
	let kind = $state<CategoryKind>('expense');
	let color = $state(DEFAULT_COLOR);
	let accountId = $state(0);

	onMount(load);

	async function load() {
		loading = true;
		loadError = null;
		try {
			[categories, accounts] = await Promise.all([getCategories(showArchived), getAccounts()]);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load categories';
		} finally {
			loading = false;
		}
	}

	let groups = $derived([
		{ label: 'Spending', kind: 'expense' as const, items: categories.filter((c) => c.kind === 'expense') },
		{ label: 'Income', kind: 'income' as const, items: categories.filter((c) => c.kind === 'income') }
	]);

	function openCreate(initialKind: CategoryKind) {
		editing = null;
		name = '';
		kind = initialKind;
		color = DEFAULT_COLOR;
		accountId = 0;
		formError = null;
		modalOpen = true;
	}

	function openEdit(category: Category) {
		editing = category;
		name = category.name;
		kind = category.kind;
		color = category.color ?? DEFAULT_COLOR;
		accountId = category.default_account_id ?? 0;
		formError = null;
		modalOpen = true;
	}

	async function handleSubmit(event: Event) {
		event.preventDefault();
		if (!name.trim()) return;

		saving = true;
		formError = null;
		notice = null;
		// parent_id has no editor yet; carry the existing value through an edit
		// rather than silently clearing it.
		const input = {
			name: name.trim(),
			kind,
			color,
			parent_id: editing?.parent_id ?? null,
			default_account_id: accountId || null
		};
		try {
			if (editing) {
				await updateCategory(editing.id, input);
			} else {
				await createCategory(input);
			}
			modalOpen = false;
			await load();
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not save the category';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!editing) return;
		const removed = editing;

		deleting = true;
		formError = null;
		try {
			await deleteCategory(removed.id);
			modalOpen = false;
			await load();
			// The API archives a category that transactions, rules, or budgets still
			// reference, so past entries keep their label. Say which one happened.
			const stillThere = (await getCategories(true)).some((c) => c.id === removed.id);
			notice = stillThere
				? `"${removed.name}" is in use, so it was archived rather than deleted. Past entries keep the label.`
				: `"${removed.name}" was deleted.`;
		} catch (e) {
			formError = e instanceof Error ? e.message : 'Could not delete the category';
		} finally {
			deleting = false;
		}
	}

	async function restore(category: Category) {
		notice = null;
		try {
			await unarchiveCategory(category.id);
			await load();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not restore the category';
		}
	}

	async function toggleShowArchived() {
		showArchived = !showArchived;
		await load();
	}
</script>

<div class="page">
	<div class="page-header">
		<div>
			<h1>Categories</h1>
			<p class="muted">What transactions, recurring items, and budgets are grouped by.</p>
		</div>
		<Button variant="ghost" size="sm" onclick={toggleShowArchived}>
			{showArchived ? 'Hide archived' : 'Show archived'}
		</Button>
	</div>

	{#if notice}
		<p class="notice">{notice}</p>
	{/if}

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if loadError}
		<p class="error-text">{loadError}</p>
	{:else}
		<div class="columns">
			{#each groups as group (group.kind)}
				<section class="card">
					<div class="section-head">
						<h2>{group.label}</h2>
						<Button size="sm" onclick={() => openCreate(group.kind)}>Add</Button>
					</div>

					{#if group.items.length === 0}
						<p class="muted">No {group.label.toLowerCase()} categories.</p>
					{:else}
						<ul class="list">
							{#each group.items as category (category.id)}
								<li class="item" class:archived={category.archived}>
									<span class="item-name">
										<span class="swatch" style="background: {category.color ?? 'var(--border)'}"></span>
										{category.name}
										{#if category.archived}
											<span class="chip">Archived</span>
										{/if}
										{#if category.default_account_id}
											{@const routed = accounts.find((a) => a.id === category.default_account_id)}
											{#if routed}<span class="chip">→ {routed.name}</span>{/if}
										{/if}
									</span>
									<span class="item-actions">
										{#if category.archived}
											<Button variant="ghost" size="sm" onclick={() => restore(category)}>Restore</Button>
										{:else}
											<Button variant="ghost" size="sm" onclick={() => openEdit(category)}>Edit</Button>
										{/if}
									</span>
								</li>
							{/each}
						</ul>
					{/if}
				</section>
			{/each}
		</div>
	{/if}
</div>

<Modal bind:open={modalOpen} title={editing ? 'Edit Category' : 'Add Category'}>
	<form onsubmit={handleSubmit}>
		<Field label="Name" id="name">
			<input id="name" bind:value={name} placeholder="Pet Care" disabled={saving} required />
		</Field>

		<div class="form-row">
			<Field label="Type" id="kind">
				<select id="kind" bind:value={kind} disabled={saving}>
					<option value="expense">Spending</option>
					<option value="income">Income</option>
				</select>
			</Field>
			<Field label="Colour" id="color" hint="Used in the spending chart">
				<input id="color" type="color" bind:value={color} disabled={saving} />
			</Field>
		</div>

		<Field
			label="Always goes on"
			id="defaultAccount"
			hint="Receipts filed here post to this account whatever card they show, and picking it on the log screen switches to it. The iPhone Shortcut ignores this."
		>
			<select id="defaultAccount" bind:value={accountId} disabled={saving}>
				<option value={0}>Whichever account it was paid from</option>
				<AccountOptions {accounts} />
			</select>
		</Field>

		{#if formError}
			<p class="error-text">{formError}</p>
		{/if}

		<div class="form-actions">
			{#if editing}
				<span class="spacer">
					<Button variant="danger" onclick={handleDelete} disabled={saving || deleting}>
						{deleting ? 'Deleting…' : 'Delete'}
					</Button>
				</span>
			{/if}
			<Button variant="secondary" onclick={() => (modalOpen = false)}>Cancel</Button>
			<Button type="submit" disabled={saving || deleting || !name.trim()}>
				{saving ? 'Saving…' : editing ? 'Save Changes' : 'Add Category'}
			</Button>
		</div>
	</form>
</Modal>

<style>
	h2 {
		font-size: 1rem;
	}

	.columns {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
		gap: 1.5rem;
		align-items: start;
	}

	.section-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
		gap: 1rem;
	}

	.notice {
		font-size: 0.875rem;
		padding: 0.625rem 0.875rem;
		border-radius: var(--radius-sm);
		background: var(--surface);
		border-left: 3px solid var(--info);
	}

	.list {
		list-style: none;
		display: flex;
		flex-direction: column;
	}

	.item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--divider);
	}

	.item:last-child {
		border-bottom: none;
	}

	.item.archived .item-name {
		color: var(--muted);
	}

	.item-name {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		font-size: 0.9375rem;
		font-weight: 500;
	}

	.item-actions {
		display: flex;
		gap: 0.25rem;
	}

	.swatch {
		width: 0.75rem;
		height: 0.75rem;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.chip {
		font-size: 0.7rem;
		font-weight: 600;
		padding: 0.1rem 0.45rem;
		border-radius: 4px;
		background: var(--bg);
		color: var(--muted);
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.form-row {
		display: flex;
		gap: 1rem;
	}

	/* Native colour inputs render a thin swatch at the shared text-input padding. */
	.form-row :global(input[type='color']) {
		height: 2.375rem;
		padding: 0.25rem;
		cursor: pointer;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.spacer {
		margin-right: auto;
	}
</style>
