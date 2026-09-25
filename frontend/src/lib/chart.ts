// The hover layer shared by the line charts: a hairline that snaps to the
// nearest date, a dot on each series there, and one tooltip listing every
// series at that date. Pointer events cover mouse and touch alike; the overlay
// is focusable and the arrow keys walk it, so the numbers aren't mouse-only.
//
// The tooltip is built with textContent, never innerHTML: series labels can be
// account names the user typed.

import * as d3 from 'd3';

export const INK = '#1a1a2e';
export const GRID = '#eee';
export const AXIS_TEXT = '#999';

/**
 * Series colours for the account charts, validated together for colour-blind
 * separation: contributions / principal, growth, interest. Each chart that uses
 * them also names them in a legend and the tooltip, never by colour alone.
 */
export const SERIES = {
	blue: '#3b7dd8',
	green: '#1e9e6f',
	orange: '#d9822b'
} as const;

export interface TipRow {
	label: string;
	value: string;
	color?: string;
	/** Draw the key as a dashed stroke, matching a projected line. */
	dashed?: boolean;
}

export interface Crosshair {
	/** Every date the pointer can snap to, ascending. */
	dates: Date[];
	x: d3.ScaleTime<number, number>;
	width: number;
	height: number;
	margin: { top: number; left: number };
	title: (i: number) => string;
	rows: (i: number) => TipRow[];
	/** Where to put a dot at this date: one per series that has a value there. */
	dots?: (i: number) => { y: number; color: string }[];
}

export function addCrosshair(
	g: d3.Selection<SVGGElement, unknown, null, undefined>,
	container: HTMLElement,
	opts: Crosshair
): void {
	const { dates, x, width, height, margin } = opts;
	if (dates.length === 0) return;

	const layer = g.append('g').style('pointer-events', 'none').style('display', 'none');
	const rule = layer
		.append('line')
		.attr('y1', 0)
		.attr('y2', height)
		.attr('stroke', INK)
		.attr('stroke-opacity', 0.35)
		.attr('stroke-width', 1);
	const dotLayer = layer.append('g');

	const tip = document.createElement('div');
	Object.assign(tip.style, {
		position: 'absolute',
		top: `${margin.top}px`,
		pointerEvents: 'none',
		display: 'none',
		background: 'var(--surface, #fff)',
		border: '1px solid var(--border, #ddd)',
		borderRadius: '8px',
		boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
		padding: '0.5rem 0.625rem',
		fontSize: '0.75rem',
		lineHeight: '1.4',
		whiteSpace: 'nowrap',
		zIndex: '2'
	} satisfies Partial<CSSStyleDeclaration>);
	container.appendChild(tip);

	const bisect = d3.bisector((d: Date) => d).center;
	let current = -1;

	function show(i: number) {
		current = i;
		const px = x(dates[i]);
		layer.style('display', null);
		rule.attr('x1', px).attr('x2', px);

		dotLayer
			.selectAll('circle')
			.data(opts.dots?.(i) ?? [])
			.join('circle')
			.attr('cx', px)
			.attr('cy', (d) => d.y)
			.attr('r', 4)
			.attr('fill', (d) => d.color)
			.attr('stroke', '#fff')
			.attr('stroke-width', 2);

		tip.replaceChildren();
		const title = document.createElement('div');
		title.textContent = opts.title(i);
		Object.assign(title.style, { color: 'var(--muted, #666)', marginBottom: '0.25rem' });
		tip.appendChild(title);
		for (const r of opts.rows(i)) {
			const row = document.createElement('div');
			Object.assign(row.style, { display: 'flex', alignItems: 'center', gap: '0.5rem' });
			const key = document.createElement('span');
			Object.assign(key.style, {
				width: '12px',
				height: '0',
				borderTop: `2px ${r.dashed ? 'dashed' : 'solid'} ${r.color ?? 'transparent'}`,
				flex: 'none'
			});
			const value = document.createElement('strong');
			value.textContent = r.value;
			Object.assign(value.style, { fontVariantNumeric: 'tabular-nums', color: 'var(--ink, #1a1a2e)' });
			const label = document.createElement('span');
			label.textContent = r.label;
			label.style.color = 'var(--muted, #666)';
			row.append(key, value, label);
			tip.appendChild(row);
		}
		tip.style.display = 'block';

		// Beside the hairline, on whichever side has room.
		const left = margin.left + px;
		const w = tip.offsetWidth;
		const onRight = left + 12 + w <= container.clientWidth;
		tip.style.left = `${onRight ? left + 12 : Math.max(0, left - 12 - w)}px`;
	}

	function hide() {
		current = -1;
		layer.style('display', 'none');
		tip.style.display = 'none';
	}

	function at(event: PointerEvent) {
		const [mx] = d3.pointer(event, g.node());
		const i = bisect(dates, x.invert(Math.max(0, Math.min(width, mx))));
		show(Math.max(0, Math.min(dates.length - 1, i)));
	}

	g.append('rect')
		.attr('width', width)
		.attr('height', height)
		.attr('fill', 'transparent')
		.attr('tabindex', 0)
		.attr('role', 'img')
		.attr('aria-label', 'Chart. Use the left and right arrow keys to read values.')
		// Vertical swipes still scroll the page on a phone; horizontal ones scrub.
		.style('touch-action', 'pan-y')
		.style('outline', 'none')
		.on('pointermove pointerdown', at)
		.on('pointerleave', hide)
		.on('blur', hide)
		.on('focus', () => show(dates.length - 1))
		.on('keydown', (event: KeyboardEvent) => {
			if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
			event.preventDefault();
			// Hold shift to move a tenth of the chart at a time.
			const step = (event.shiftKey ? Math.max(1, Math.round(dates.length / 10)) : 1) * (event.key === 'ArrowLeft' ? -1 : 1);
			show(Math.max(0, Math.min(dates.length - 1, (current < 0 ? dates.length - 1 : current) + step)));
		});
}

/** Axis labels as "$4.4k" — or whole dollars when the range is too narrow for that to differ. */
export function moneyTicks(lo: number, hi: number, whole: (n: number) => string): (d: d3.NumberValue) => string {
	if (hi - lo < 5000) return (d) => whole(d as number);
	return (d) => {
		const n = d as number;
		return `${n < 0 ? '−' : ''}$${d3.format('.3~s')(Math.abs(n)).replace('G', 'B')}`;
	};
}

/** Recessive axis styling, shared so every chart reads the same. */
export function styleAxis(sel: d3.Selection<SVGGElement, unknown, null, undefined>): void {
	sel.selectAll('text').attr('fill', AXIS_TEXT).attr('font-size', '0.7rem');
	sel.selectAll('.domain').attr('stroke', '#ddd');
	sel.selectAll('.tick line').attr('stroke', '#ddd');
}
