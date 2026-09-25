<script lang="ts">
	import * as d3 from 'd3';
	import { formatCurrency, formatCurrencyWhole, formatDate, parseDate } from '$lib/format';
	import { INK, addCrosshair, moneyTicks, styleAxis, type TipRow } from '$lib/chart';

	// What has happened, then what would happen: an account's real history as a
	// solid line up to today, joined to a projection drawn from today on. The
	// projection can be stacked areas (what you put in vs what it earned), dashed
	// lines (two payoff plans side by side) and a shaded band (a range of
	// returns). One hairline reads every series at the date under the pointer.

	type Row = { date: string } & Record<string, number | string>;
	interface Series {
		key: string;
		label: string;
		color: string;
	}

	let {
		history = [],
		historyLabel = 'So far',
		rows,
		stack = [],
		lines = [],
		band = null,
		height: chartHeight = 300
	}: {
		/** Actual values up to today, oldest first. */
		history?: { date: string; value: number }[];
		historyLabel?: string;
		/** The projection, one row per month, starting today. */
		rows: Row[];
		/** Keys stacked as filled areas from zero, bottom first. */
		stack?: Series[];
		/** Keys drawn as dashed lines. */
		lines?: Series[];
		/** A shaded range between two keys. */
		band?: (Series & { lo: string; hi: string }) | null;
		height?: number;
	} = $props();

	let container: HTMLDivElement;
	let boxWidth = $state(0);

	const num = (r: Row, key: string) => Number(r[key]);

	function render() {
		if (!container) return;
		d3.select(container).selectAll('*').remove();
		if (rows.length < 2) return;

		const margin = { top: 16, right: 16, bottom: 32, left: 64 };
		const width = container.clientWidth - margin.left - margin.right;
		const height = chartHeight - margin.top - margin.bottom;
		if (width <= 0) return;

		const hist = history.map((d) => ({ date: parseDate(d.date), value: d.value }));
		const proj = rows.map((r) => ({ date: parseDate(r.date), row: r }));

		const svg = d3
			.select(container)
			.append('svg')
			.attr('width', width + margin.left + margin.right)
			.attr('height', height + margin.top + margin.bottom)
			.append('g')
			.attr('transform', `translate(${margin.left},${margin.top})`);

		const first = hist.length ? hist[0].date : proj[0].date;
		const x = d3.scaleTime().domain([first, proj[proj.length - 1].date]).range([0, width]);

		const values: number[] = [0, ...hist.map((d) => d.value)];
		for (const { row } of proj) {
			let top = 0;
			for (const s of stack) top += num(row, s.key);
			values.push(top);
			for (const s of lines) values.push(num(row, s.key));
			if (band) values.push(num(row, band.lo), num(row, band.hi));
		}
		const y = d3
			.scaleLinear()
			.domain(d3.extent(values) as [number, number])
			.nice()
			.range([height, 0]);

		const spanDays = (x.domain()[1].getTime() - x.domain()[0].getTime()) / 86_400_000;
		const tickFormat = spanDays > 3 * 365 ? '%Y' : spanDays > 120 ? "%b '%y" : '%b %d';
		svg
			.append('g')
			.attr('transform', `translate(0,${height})`)
			.call(
				d3
					.axisBottom(x)
					.ticks(Math.min(7, Math.max(2, Math.floor(width / 80))))
					.tickFormat(d3.timeFormat(tickFormat) as any)
			)
			.call(styleAxis);

		const [lo, hi] = y.domain();
		svg
			.append('g')
			.call(
				d3
					.axisLeft(y)
					.ticks(5)
					.tickSize(-width)
					.tickFormat(moneyTicks(lo, hi, formatCurrencyWhole))
			)
			.call(styleAxis)
			.call((g) => g.selectAll('.tick line').attr('stroke', '#f0f0f0'))
			.call((g) => g.select('.domain').remove());

		if (lo < 0 && hi > 0) {
			svg
				.append('line')
				.attr('x1', 0)
				.attr('x2', width)
				.attr('y1', y(0))
				.attr('y2', y(0))
				.attr('stroke', '#ccc');
		}

		// Stacked areas, each sitting on the ones before it. A 2px surface
		// stroke keeps neighbouring fills apart.
		const stacked = proj.map(({ date, row }) => {
			let base = 0;
			const layers = stack.map((s) => {
				const v = num(row, s.key);
				const layer = { y0: base, y1: base + v };
				base += v;
				return layer;
			});
			return { date, layers };
		});
		stack.forEach((s, i) => {
			svg
				.append('path')
				.datum(stacked)
				.attr('fill', s.color)
				.attr('fill-opacity', 0.28)
				.attr('stroke', '#fff')
				.attr('stroke-width', 2)
				.attr(
					'd',
					d3
						.area<(typeof stacked)[number]>()
						.x((d) => x(d.date))
						.y0((d) => y(d.layers[i].y0))
						.y1((d) => y(d.layers[i].y1))
				);
		});
		if (stack.length) {
			const top = stack[stack.length - 1];
			svg
				.append('path')
				.datum(stacked)
				.attr('fill', 'none')
				.attr('stroke', top.color)
				.attr('stroke-width', 2)
				.attr(
					'd',
					d3
						.line<(typeof stacked)[number]>()
						.x((d) => x(d.date))
						.y((d) => y(d.layers[stack.length - 1].y1))
				);
		}

		if (band) {
			const b = band;
			svg
				.append('path')
				.datum(proj)
				.attr('fill', b.color)
				.attr('fill-opacity', 0.1)
				.attr(
					'd',
					d3
						.area<(typeof proj)[number]>()
						.x((d) => x(d.date))
						.y0((d) => y(num(d.row, b.lo)))
						.y1((d) => y(num(d.row, b.hi)))
				);
			for (const key of [b.lo, b.hi]) {
				svg
					.append('path')
					.datum(proj)
					.attr('fill', 'none')
					.attr('stroke', b.color)
					.attr('stroke-opacity', 0.5)
					.attr('stroke-width', 1)
					.attr('d', d3.line<(typeof proj)[number]>().x((d) => x(d.date)).y((d) => y(num(d.row, key))));
			}
		}

		for (const s of lines) {
			svg
				.append('path')
				.datum(proj)
				.attr('fill', 'none')
				.attr('stroke', s.color)
				.attr('stroke-width', 2)
				.attr('stroke-dasharray', '6,4')
				.attr('d', d3.line<(typeof proj)[number]>().x((d) => x(d.date)).y((d) => y(num(d.row, s.key))));
		}

		if (hist.length >= 2) {
			svg
				.append('path')
				.datum(hist)
				.attr('fill', 'none')
				.attr('stroke', INK)
				.attr('stroke-width', 2)
				.attr('d', d3.line<(typeof hist)[number]>().x((d) => x(d.date)).y((d) => y(d.value)));
		}

		// Where history ends and the projection starts.
		const now = x(proj[0].date);
		if (now > 0) {
			svg
				.append('line')
				.attr('x1', now)
				.attr('x2', now)
				.attr('y1', 0)
				.attr('y2', height)
				.attr('stroke', INK)
				.attr('stroke-opacity', 0.25)
				.attr('stroke-dasharray', '2,3');
			svg
				.append('text')
				.attr('x', now + 4)
				.attr('y', 10)
				.attr('font-size', '0.7rem')
				.attr('fill', '#999')
				.text('Today');
		}

		// Every date the hairline can land on: the history's, then the
		// projection's after today (its first row is today, already covered).
		type Stop = { date: Date; h?: number; p?: number };
		const stops: Stop[] = hist.map((d, i) => ({ date: d.date, h: i }));
		proj.forEach((d, i) => {
			if (i === 0 && stops.length) stops[stops.length - 1].p = 0;
			else stops.push({ date: d.date, p: i });
		});

		addCrosshair(svg, container, {
			dates: stops.map((s) => s.date),
			x,
			width,
			height,
			margin,
			title: (i) => {
				const s = stops[i];
				const date = formatDate(s.p != null ? rows[s.p].date : history[s.h!].date);
				return s.p != null && s.p > 0 ? `${date} · projected` : date;
			},
			rows: (i) => {
				const s = stops[i];
				if (s.p == null || (s.p === 0 && s.h != null && !stack.length && !lines.length)) {
					return [{ label: historyLabel, value: formatCurrency(hist[s.h!].value), color: INK }];
				}
				const row = rows[s.p];
				const out: TipRow[] = [];
				for (const l of lines) out.push({ label: l.label, value: formatCurrency(num(row, l.key)), color: l.color, dashed: true });
				if (stack.length) {
					const total = stack.reduce((sum, st) => sum + num(row, st.key), 0);
					out.push({ label: 'total', value: formatCurrency(total) });
					for (const st of [...stack].reverse()) {
						out.push({ label: st.label, value: formatCurrency(num(row, st.key)), color: st.color });
					}
				}
				if (band) {
					out.push({
						label: band.label,
						value: `${formatCurrencyWhole(num(row, band.lo))} – ${formatCurrencyWhole(num(row, band.hi))}`,
						color: band.color
					});
				}
				return out;
			},
			dots: (i) => {
				const s = stops[i];
				if (s.p == null) return [{ y: y(hist[s.h!].value), color: INK }];
				const row = rows[s.p];
				const dots = lines.map((l) => ({ y: y(num(row, l.key)), color: l.color }));
				if (stack.length) {
					const total = stack.reduce((sum, st) => sum + num(row, st.key), 0);
					dots.push({ y: y(total), color: stack[stack.length - 1].color });
				}
				return dots;
			}
		});
	}

	$effect(() => {
		rows;
		history;
		stack;
		lines;
		band;
		boxWidth;
		render();
	});
</script>

<div bind:this={container} bind:clientWidth={boxWidth} class="chart" style:min-height="{chartHeight}px"></div>
<ul class="legend">
	{#if history.length >= 2}
		<li><span class="key line" style:border-color={INK}></span>{historyLabel}</li>
	{/if}
	{#each stack as s (s.key)}
		<li><span class="key area" style:background={s.color}></span>{s.label}</li>
	{/each}
	{#each lines as s (s.key)}
		<li><span class="key line dashed" style:border-color={s.color}></span>{s.label}</li>
	{/each}
	{#if band}
		<li><span class="key area faint" style:background={band.color}></span>{band.label}</li>
	{/if}
</ul>

<style>
	.chart {
		position: relative;
		width: 100%;
	}

	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
		font-size: 0.75rem;
		color: var(--muted);
	}

	.legend li {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.key {
		display: inline-block;
		width: 14px;
		flex: none;
	}

	.key.line {
		height: 0;
		border-top: 2px solid;
	}

	.key.line.dashed {
		border-top-style: dashed;
	}

	.key.area {
		height: 10px;
		border-radius: 2px;
		opacity: 0.45;
	}

	.key.area.faint {
		opacity: 0.18;
	}
</style>
