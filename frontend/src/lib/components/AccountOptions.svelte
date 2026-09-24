<!--
	The <option>s for an account <select>, grouped the way this device groups
	accounts. Goes inside the page's own <select>, so leading choices like
	"All accounts" stay with the page. One group needs no heading.
-->
<script lang="ts">
	import type { Account } from '$lib/types';
	import { groupAccounts } from '$lib/grouping';
	import { grouping } from '$lib/grouping.svelte';

	let { accounts, showMask = false }: { accounts: Account[]; showMask?: boolean } = $props();

	const text = (a: Account) => a.name + (showMask && a.mask ? ` ····${a.mask}` : '');

	let groups = $derived(groupAccounts(accounts, grouping.by));
</script>

{#if groups.length > 1}
	{#each groups as g (g.key)}
		<optgroup label={g.label}>
			{#each g.items as a (a.id)}
				<option value={a.id}>{text(a)}</option>
			{/each}
		</optgroup>
	{/each}
{:else}
	{#each accounts as a (a.id)}
		<option value={a.id}>{text(a)}</option>
	{/each}
{/if}
