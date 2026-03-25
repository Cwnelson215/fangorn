<script lang="ts">
	import { onMount } from 'svelte';
	import * as d3 from 'd3';
	import type { Transaction } from '$lib/types';

	let { transactions }: { transactions: Transaction[] } = $props();
	let container: HTMLDivElement;

	function render() {
		if (!container || transactions.length === 0) return;

		d3.select(container).selectAll('*').remove();

		// Aggregate by week
		const byWeek = new Map<string, number>();
		for (const txn of transactions) {
			if (txn.amount <= 0) continue; // only expenses
			const date = new Date(txn.date);
			const week = d3.timeWeek.floor(date);
			const key = week.toISOString().slice(0, 10);
			byWeek.set(key, (byWeek.get(key) || 0) + txn.amount);
		}

		const data = Array.from(byWeek.entries())
			.map(([date, amount]) => ({ date: new Date(date), amount }))
			.sort((a, b) => a.date.getTime() - b.date.getTime());

		if (data.length < 2) return;

		const margin = { top: 20, right: 20, bottom: 40, left: 60 };
		const width = container.clientWidth - margin.left - margin.right;
		const height = 250 - margin.top - margin.bottom;

		const svg = d3.select(container)
			.append('svg')
			.attr('width', width + margin.left + margin.right)
			.attr('height', height + margin.top + margin.bottom)
			.append('g')
			.attr('transform', `translate(${margin.left},${margin.top})`);

		const x = d3.scaleTime()
			.domain(d3.extent(data, d => d.date) as [Date, Date])
			.range([0, width]);

		const y = d3.scaleLinear()
			.domain([0, d3.max(data, d => d.amount) || 0])
			.nice()
			.range([height, 0]);

		svg.append('g')
			.attr('transform', `translate(0,${height})`)
			.call(d3.axisBottom(x).ticks(5).tickFormat(d3.timeFormat('%b %d') as any))
			.selectAll('text')
			.attr('fill', '#999')
			.attr('font-size', '0.7rem');

		svg.append('g')
			.call(d3.axisLeft(y).ticks(5).tickFormat(d => `$${d3.format('.0s')(d as number)}`))
			.selectAll('text')
			.attr('fill', '#999')
			.attr('font-size', '0.7rem');

		const line = d3.line<{ date: Date; amount: number }>()
			.x(d => x(d.date))
			.y(d => y(d.amount))
			.curve(d3.curveMonotoneX);

		const area = d3.area<{ date: Date; amount: number }>()
			.x(d => x(d.date))
			.y0(height)
			.y1(d => y(d.amount))
			.curve(d3.curveMonotoneX);

		svg.append('path')
			.datum(data)
			.attr('fill', 'rgba(78, 204, 163, 0.1)')
			.attr('d', area);

		svg.append('path')
			.datum(data)
			.attr('fill', 'none')
			.attr('stroke', '#4ecca3')
			.attr('stroke-width', 2)
			.attr('d', line);
	}

	onMount(render);
	$effect(() => { transactions; render(); });
</script>

<div bind:this={container} class="chart"></div>

<style>
	.chart {
		width: 100%;
		min-height: 250px;
	}
</style>
