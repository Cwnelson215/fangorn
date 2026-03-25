<script lang="ts">
	import { onMount } from 'svelte';
	import * as d3 from 'd3';
	import type { NetWorthPoint } from '$lib/types';

	let { data }: { data: NetWorthPoint[] } = $props();
	let container: HTMLDivElement;

	function formatCurrency(n: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 }).format(n);
	}

	function render() {
		if (!container || data.length < 2) return;

		d3.select(container).selectAll('*').remove();

		const parsed = data.map(d => ({ date: new Date(d.date), value: d.net_worth }));

		const margin = { top: 20, right: 20, bottom: 40, left: 70 };
		const width = container.clientWidth - margin.left - margin.right;
		const height = 280 - margin.top - margin.bottom;

		const svg = d3.select(container)
			.append('svg')
			.attr('width', width + margin.left + margin.right)
			.attr('height', height + margin.top + margin.bottom)
			.append('g')
			.attr('transform', `translate(${margin.left},${margin.top})`);

		const x = d3.scaleTime()
			.domain(d3.extent(parsed, d => d.date) as [Date, Date])
			.range([0, width]);

		const [yMin, yMax] = d3.extent(parsed, d => d.value) as [number, number];
		const padding = (yMax - yMin) * 0.1 || 100;
		const y = d3.scaleLinear()
			.domain([yMin - padding, yMax + padding])
			.range([height, 0]);

		svg.append('g')
			.attr('transform', `translate(0,${height})`)
			.call(d3.axisBottom(x).ticks(6).tickFormat(d3.timeFormat('%b %d') as any))
			.selectAll('text')
			.attr('fill', '#999')
			.attr('font-size', '0.7rem');

		svg.append('g')
			.call(d3.axisLeft(y).ticks(5).tickFormat(d => `$${d3.format('.2s')(d as number)}`))
			.selectAll('text')
			.attr('fill', '#999')
			.attr('font-size', '0.7rem');

		// Zero line if range spans 0
		if (yMin < 0 && yMax > 0) {
			svg.append('line')
				.attr('x1', 0).attr('x2', width)
				.attr('y1', y(0)).attr('y2', y(0))
				.attr('stroke', '#ddd')
				.attr('stroke-dasharray', '4,4');
		}

		const area = d3.area<{ date: Date; value: number }>()
			.x(d => x(d.date))
			.y0(height)
			.y1(d => y(d.value))
			.curve(d3.curveMonotoneX);

		const line = d3.line<{ date: Date; value: number }>()
			.x(d => x(d.date))
			.y(d => y(d.value))
			.curve(d3.curveMonotoneX);

		// Gradient fill
		const gradient = svg.append('defs')
			.append('linearGradient')
			.attr('id', 'nw-gradient')
			.attr('x1', '0').attr('y1', '0')
			.attr('x2', '0').attr('y2', '1');
		gradient.append('stop').attr('offset', '0%').attr('stop-color', '#45b7d1').attr('stop-opacity', 0.3);
		gradient.append('stop').attr('offset', '100%').attr('stop-color', '#45b7d1').attr('stop-opacity', 0.02);

		svg.append('path')
			.datum(parsed)
			.attr('fill', 'url(#nw-gradient)')
			.attr('d', area);

		svg.append('path')
			.datum(parsed)
			.attr('fill', 'none')
			.attr('stroke', '#45b7d1')
			.attr('stroke-width', 2.5)
			.attr('d', line);

		// Latest value label
		const latest = parsed[parsed.length - 1];
		svg.append('circle')
			.attr('cx', x(latest.date))
			.attr('cy', y(latest.value))
			.attr('r', 4)
			.attr('fill', '#45b7d1');

		svg.append('text')
			.attr('x', x(latest.date) - 5)
			.attr('y', y(latest.value) - 12)
			.attr('text-anchor', 'end')
			.attr('font-size', '0.8rem')
			.attr('font-weight', '600')
			.attr('fill', '#45b7d1')
			.text(formatCurrency(latest.value));
	}

	onMount(render);
	$effect(() => { data; render(); });
</script>

<div bind:this={container} class="chart"></div>

<style>
	.chart {
		width: 100%;
		min-height: 280px;
	}
</style>
