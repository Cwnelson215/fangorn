<script lang="ts">
	import * as d3 from 'd3';
	import { formatCurrency, formatCurrencyWhole, formatDate, parseDate } from '$lib/format';
	import { ACCENT, AXIS_LINE, AXIS_TEXT, addCrosshair, moneyTicks, styleAxis, type TipRow } from '$lib/chart';

	// A dated line of dollar values: net worth snapshots, an account's value, what
	// a card owes. Hovering (or scrubbing with a finger) reads off any day; an
	// investment account's points also carry the cash / invested split, which the
	// tooltip shows beneath the total.
	//
	// The gradient id must be unique per instance: two charts on one page would
	// otherwise both reference the same <linearGradient> and the second would
	// silently restyle the first.
	let {
		points,
		id,
		label = 'Value'
	}: {
		points: { date: string; value: number; cash?: number; holdings?: number }[];
		id: string;
		/** What the line is, in the tooltip: "Net worth", "Owed". */
		label?: string;
	} = $props();
	let container: HTMLDivElement;
	// Redrawn whenever the box changes size: a phone rotating, a window resizing.
	let boxWidth = $state(0);
	let gradientId = $derived(`${id}-gradient`);

	function render() {
		if (!container) return;
		d3.select(container).selectAll('*').remove();
		if (points.length < 2) return;

		const parsed = points.map((d) => ({ date: parseDate(d.date), value: d.value }));
		// Only split the value when there's something invested to split it from.
		const split = points.some((p) => (p.holdings ?? 0) !== 0);

		const margin = { top: 20, right: 20, bottom: 40, left: 70 };
		const width = container.clientWidth - margin.left - margin.right;
		const height = 280 - margin.top - margin.bottom;

		const svg = d3.select(container)
			.append('svg')
			.attr('width', width + margin.left + margin.right)
			.attr('height', height + margin.top + margin.bottom)
			.append('g')
			.attr('transform', `translate(${margin.left},${margin.top})`);

		// Past a few months, day-of-month labels stop meaning much and a year
		// boundary becomes ambiguous.
		const spanDays = (parsed[parsed.length - 1].date.getTime() - parsed[0].date.getTime()) / 86_400_000;
		const tickFormat = spanDays > 120 ? "%b '%y" : '%b %d';

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
			.call(d3.axisBottom(x).ticks(Math.min(6, Math.max(2, Math.floor(width / 70)))).tickFormat(d3.timeFormat(tickFormat) as any))
			.selectAll('text')
			.attr('fill', AXIS_TEXT)
			.attr('font-size', '0.7rem');

		// SI-abbreviated labels ("$4.4k") collapse into duplicates when the range
		// is narrow — an investment account moving a few hundred dollars — so
		// show whole dollars there.
		const [lo, hi] = y.domain();
		svg.append('g')
			.call(d3.axisLeft(y).ticks(5).tickFormat(moneyTicks(lo, hi, formatCurrencyWhole)))
			.call(styleAxis);

		// Zero line if range spans 0
		if (yMin < 0 && yMax > 0) {
			svg.append('line')
				.attr('x1', 0).attr('x2', width)
				.attr('y1', y(0)).attr('y2', y(0))
				.attr('stroke', AXIS_LINE)
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
			.attr('id', gradientId)
			.attr('x1', '0').attr('y1', '0')
			.attr('x2', '0').attr('y2', '1');
		gradient.append('stop').attr('offset', '0%').attr('stop-color', ACCENT).attr('stop-opacity', 0.3);
		gradient.append('stop').attr('offset', '100%').attr('stop-color', ACCENT).attr('stop-opacity', 0.02);

		svg.append('path')
			.datum(parsed)
			.attr('fill', `url(#${gradientId})`)
			.attr('d', area);

		svg.append('path')
			.datum(parsed)
			.attr('fill', 'none')
			.attr('stroke', ACCENT)
			.attr('stroke-width', 2.5)
			.attr('d', line);

		// Latest value label
		const latest = parsed[parsed.length - 1];
		svg.append('circle')
			.attr('cx', x(latest.date))
			.attr('cy', y(latest.value))
			.attr('r', 4)
			.attr('fill', ACCENT);

		svg.append('text')
			.attr('x', x(latest.date) - 5)
			.attr('y', y(latest.value) - 12)
			.attr('text-anchor', 'end')
			.attr('font-size', '0.8rem')
			.attr('font-weight', '600')
			.attr('fill', ACCENT)
			.text(formatCurrencyWhole(latest.value));

		addCrosshair(svg, container, {
			dates: parsed.map((d) => d.date),
			x,
			width,
			height,
			margin,
			title: (i) => formatDate(points[i].date),
			rows: (i) => {
				const p = points[i];
				const rows: TipRow[] = [{ label, value: formatCurrency(p.value), color: ACCENT }];
				if (split) {
					rows.push({ label: 'invested', value: formatCurrency(p.holdings ?? 0) });
					rows.push({ label: 'cash', value: formatCurrency(p.cash ?? 0) });
				}
				return rows;
			},
			dots: (i) => [{ y: y(parsed[i].value), color: ACCENT }]
		});
	}

	$effect(() => { points; boxWidth; render(); });
</script>

<div bind:this={container} bind:clientWidth={boxWidth} class="chart"></div>

<style>
	.chart {
		position: relative;
		width: 100%;
		min-height: 280px;
	}
</style>
