<script lang="ts">
	import { onMount } from 'svelte';
	import * as d3 from 'd3';
	import type { CategoryBreakdown } from '$lib/types';

	let { data }: { data: CategoryBreakdown[] } = $props();
	let container: HTMLDivElement;

	const COLORS = [
		'#4ecca3', '#ff6b6b', '#4ecdc4', '#45b7d1', '#96ceb4',
		'#ffeaa7', '#dfe6e9', '#fd79a8', '#a29bfe', '#55a3f0'
	];

	function formatCurrency(n: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 }).format(n);
	}

	function render() {
		if (!container || data.length === 0) return;

		d3.select(container).selectAll('*').remove();

		const width = container.clientWidth;
		const height = 300;
		const radius = Math.min(width * 0.4, height * 0.45);

		const svg = d3.select(container)
			.append('svg')
			.attr('width', width)
			.attr('height', height);

		const g = svg.append('g')
			.attr('transform', `translate(${width * 0.35}, ${height / 2})`);

		const pie = d3.pie<CategoryBreakdown>()
			.value(d => d.amount)
			.sort(null);

		const arc = d3.arc<d3.PieArcDatum<CategoryBreakdown>>()
			.innerRadius(radius * 0.55)
			.outerRadius(radius);

		const arcs = g.selectAll('.arc')
			.data(pie(data))
			.enter()
			.append('g');

		arcs.append('path')
			.attr('d', arc)
			.attr('fill', (_, i) => COLORS[i % COLORS.length])
			.attr('stroke', 'white')
			.attr('stroke-width', 2);

		// Total in center
		const total = data.reduce((sum, d) => sum + d.amount, 0);
		g.append('text')
			.attr('text-anchor', 'middle')
			.attr('dy', '-0.2em')
			.attr('font-size', '0.8rem')
			.attr('fill', '#999')
			.text('Total');
		g.append('text')
			.attr('text-anchor', 'middle')
			.attr('dy', '1em')
			.attr('font-size', '1.1rem')
			.attr('font-weight', '700')
			.attr('fill', '#1a1a2e')
			.text(formatCurrency(total));

		// Legend
		const legend = svg.append('g')
			.attr('transform', `translate(${width * 0.7}, 20)`);

		const items = legend.selectAll('.legend-item')
			.data(data.slice(0, 8))
			.enter()
			.append('g')
			.attr('transform', (_, i) => `translate(0, ${i * 28})`);

		items.append('rect')
			.attr('width', 12)
			.attr('height', 12)
			.attr('rx', 2)
			.attr('fill', (_, i) => COLORS[i % COLORS.length]);

		items.append('text')
			.attr('x', 18)
			.attr('y', 10)
			.attr('font-size', '0.75rem')
			.attr('fill', '#666')
			.text(d => {
				const label = d.category.length > 14 ? d.category.slice(0, 14) + '...' : d.category;
				return `${label} ${formatCurrency(d.amount)}`;
			});
	}

	onMount(render);
	$effect(() => { data; render(); });
</script>

<div bind:this={container} class="chart"></div>

<style>
	.chart {
		width: 100%;
		min-height: 300px;
	}
</style>
