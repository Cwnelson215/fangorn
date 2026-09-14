<script lang="ts">
	import { onMount } from 'svelte';
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
	let container: HTMLDivElement;

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

	function render() {
		if (!container) return;
		d3.select(container).selectAll('*').remove();
		const data = slices.filter((s) => s.value > 0);
		if (data.length === 0) return;

		const width = container.clientWidth;
		const height = 300;
		const radius = Math.min(width * 0.4, height * 0.45);

		const svg = d3.select(container).append('svg').attr('width', width).attr('height', height);

		const g = svg.append('g').attr('transform', `translate(${width * 0.35}, ${height / 2})`);

		const pie = d3
			.pie<Slice>()
			.value((d) => d.value)
			.sort(null);

		const arc = d3
			.arc<d3.PieArcDatum<Slice>>()
			.innerRadius(radius * 0.55)
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

		const total = data.reduce((sum, d) => sum + d.value, 0);
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

		const legend = svg.append('g').attr('transform', `translate(${width * 0.7}, 20)`);

		const items = legend
			.selectAll('.legend-item')
			.data(data.slice(0, 8))
			.enter()
			.append('g')
			.attr('transform', (_, i) => `translate(0, ${i * 28})`);

		items
			.append('rect')
			.attr('width', 12)
			.attr('height', 12)
			.attr('rx', 2)
			.attr('fill', (d, i) => colorFor(d, i));

		items
			.append('text')
			.attr('x', 18)
			.attr('y', 10)
			.attr('font-size', '0.75rem')
			.attr('fill', '#666')
			.text((d) => {
				const label = d.label.length > 14 ? d.label.slice(0, 14) + '…' : d.label;
				return `${label} ${legendValue(d, total)}`;
			});
	}

	onMount(render);
	$effect(() => {
		slices;
		render();
	});
</script>

<div bind:this={container} class="chart"></div>

<style>
	.chart {
		width: 100%;
		min-height: 300px;
	}
</style>
