<script lang="ts">
	import * as d3 from 'd3';
	import type { Slice } from '$lib/types';
	import { formatCurrencyWhole } from '$lib/format';

	// A share-of-total donut with a legend: spending by category, portfolio
	// allocation. Slices of zero or less are left out — a pie can't draw them.
	let {
		slices,
		centerLabel = 'Total',
		legendValue = (s) => formatCurrencyWhole(s.value)
	}: {
		slices: Slice[];
		centerLabel?: string;
		legendValue?: (slice: Slice, total: number) => string;
	} = $props();
	let container = $state<HTMLDivElement>();

	// Fallback palette for categories with no colour of their own.
	const COLORS = [
		'#4ecca3',
		'#ff6b6b',
		'#4ecdc4',
		'#45b7d1',
		'#96ceb4',
		'#ffeaa7',
		'#dfe6e9',
		'#fd79a8',
		'#a29bfe',
		'#55a3f0'
	];

	const colorFor = (d: Slice, i: number) => d.color || COLORS[i % COLORS.length];

	let data = $derived(slices.filter((s) => s.value > 0));
	let total = $derived(data.reduce((sum, d) => sum + d.value, 0));

	// The legend is HTML beside the ring, so it can wrap underneath on a phone
	// instead of being clipped at the edge of a fixed-size SVG.
	const SIZE = 220;

	function render() {
		if (!container) return;
		d3.select(container).selectAll('*').remove();
		if (data.length === 0) return;

		const radius = SIZE / 2 - 4;
		const svg = d3
			.select(container)
			.append('svg')
			.attr('viewBox', `0 0 ${SIZE} ${SIZE}`)
			.attr('width', SIZE)
			.attr('height', SIZE);

		const g = svg.append('g').attr('transform', `translate(${SIZE / 2}, ${SIZE / 2})`);

		const pie = d3
			.pie<Slice>()
			.value((d) => d.value)
			.sort(null);

		const arc = d3
			.arc<d3.PieArcDatum<Slice>>()
			.innerRadius(radius * 0.62)
			.outerRadius(radius);

		g.selectAll('.arc')
			.data(pie(data))
			.enter()
			.append('g')
			.append('path')
			.attr('d', arc)
			.attr('fill', (d, i) => colorFor(d.data, i))
			.attr('stroke', 'white')
			.attr('stroke-width', 2);

		g.append('text')
			.attr('text-anchor', 'middle')
			.attr('dy', '-0.2em')
			.attr('font-size', '0.8rem')
			.attr('fill', '#999')
			.text(centerLabel);
		g.append('text')
			.attr('text-anchor', 'middle')
			.attr('dy', '1em')
			.attr('font-size', '1.1rem')
			.attr('font-weight', '700')
			.attr('fill', '#1a1a2e')
			.text(formatCurrencyWhole(total));
	}

	$effect(() => {
		data;
		render();
	});
</script>

{#if data.length > 0}
	<div class="donut">
		<div bind:this={container} class="ring"></div>
		<ul class="legend">
			{#each data.slice(0, 8) as slice, i (slice.label)}
				<li>
					<span class="swatch" style="background: {colorFor(slice, i)}"></span>
					<span class="label">{slice.label}</span>
					<span class="value">{legendValue(slice, total)}</span>
				</li>
			{/each}
		</ul>
	</div>
{/if}

<style>
	.donut {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		gap: 1rem 2rem;
	}

	.ring {
		flex: none;
		width: 220px;
		height: 220px;
	}

	.legend {
		list-style: none;
		flex: 1 1 12rem;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		font-size: 0.8125rem;
		min-width: 0;
	}

	.legend li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.swatch {
		width: 12px;
		height: 12px;
		border-radius: 2px;
		flex: none;
	}

	.label {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--muted);
	}

	.value {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
</style>
