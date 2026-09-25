<script lang="ts">
	import * as d3 from 'd3';
	import type { WeekSpend } from '$lib/types';
	import { parseDate } from '$lib/format';
	import { ACCENT, AXIS_TEXT } from '$lib/chart';

	// Weekly totals arrive already bucketed from the dashboard endpoint, with
	// empty weeks as zeros. Summing raw transactions here instead would silently
	// undercount once a period held more rows than the transactions API returns.
	let { data: weeks }: { data: WeekSpend[] } = $props();
	let container: HTMLDivElement;
	// Redrawn whenever the box changes size: a phone rotating, a window resizing.
	let boxWidth = $state(0);

	function render() {
		if (!container) return;
		d3.select(container).selectAll('*').remove();

		const data = weeks.map((w) => ({ date: parseDate(w.week), amount: w.amount }));
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

		// The floor is 0 rather than the minimum, so an ordinary week is measured
		// against zero — but a week whose refunds outweigh its spending is negative
		// and has to fit, or it would be drawn below the axis and clipped.
		const y = d3.scaleLinear()
			.domain([Math.min(0, d3.min(data, d => d.amount) ?? 0), d3.max(data, d => d.amount) || 0])
			.nice()
			.range([height, 0]);

		svg.append('g')
			.attr('transform', `translate(0,${height})`)
			.call(d3.axisBottom(x).ticks(Math.min(5, Math.max(2, Math.floor(width / 70)))).tickFormat(d3.timeFormat('%b %d') as any))
			.selectAll('text')
			.attr('fill', AXIS_TEXT)
			.attr('font-size', '0.7rem');

		svg.append('g')
			.call(d3.axisLeft(y).ticks(5).tickFormat(d => `$${d3.format('.0s')(d as number)}`))
			.selectAll('text')
			.attr('fill', AXIS_TEXT)
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
			.attr('fill', ACCENT)
			.attr('fill-opacity', 0.12)
			.attr('d', area);

		svg.append('path')
			.datum(data)
			.attr('fill', 'none')
			.attr('stroke', ACCENT)
			.attr('stroke-width', 2)
			.attr('d', line);
	}

	$effect(() => { weeks; boxWidth; render(); });
</script>

<div bind:this={container} bind:clientWidth={boxWidth} class="chart"></div>

<style>
	.chart {
		width: 100%;
		min-height: 250px;
	}
</style>
