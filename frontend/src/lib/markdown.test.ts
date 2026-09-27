import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './markdown';
import { parseEvent } from './api';

describe('renderMarkdown', () => {
	it('escapes everything the model writes', () => {
		const html = renderMarkdown('Note: <img src=x onerror=alert(1)> & "quotes"');
		expect(html).toBe('<p>Note: &lt;img src=x onerror=alert(1)&gt; &amp; &quot;quotes&quot;</p>');
	});

	it('does not let markup through bold or code', () => {
		expect(renderMarkdown('**<b>hi</b>** `<script>`')).toBe(
			'<p><strong>&lt;b&gt;hi&lt;/b&gt;</strong> <code>&lt;script&gt;</code></p>'
		);
	});

	it('renders paragraphs, line breaks and emphasis', () => {
		expect(renderMarkdown('You spent **$412.50** on *groceries*.\nThat is up.\n\nNext.')).toBe(
			'<p>You spent <strong>$412.50</strong> on <em>groceries</em>.<br>That is up.</p><p>Next.</p>'
		);
	});

	it('leaves underscores and lone stars alone', () => {
		expect(renderMarkdown('high_yield_savings costs 2 * 3')).toBe('<p>high_yield_savings costs 2 * 3</p>');
	});

	it('renders bullet and numbered lists', () => {
		expect(renderMarkdown('Top spending:\n- Groceries: $400\n- Gas: $120\n\n1. First\n2. Second')).toBe(
			'<p>Top spending:</p><ul><li>Groceries: $400</li><li>Gas: $120</li></ul><ol><li>First</li><li>Second</li></ol>'
		);
	});

	it('renders tables with numeric columns right-aligned', () => {
		const html = renderMarkdown('| Month | Spent |\n|---|---:|\n| Jan | $1,200.00 |\n| Feb | **$980.10** |');
		expect(html).toBe(
			'<div class="md-table"><table><thead><tr><th>Month</th><th class="num">Spent</th></tr></thead>' +
				'<tbody><tr><td>Jan</td><td class="num">$1,200.00</td></tr>' +
				'<tr><td>Feb</td><td class="num"><strong>$980.10</strong></td></tr></tbody></table></div>'
		);
	});

	it('keeps headings small and drops rules', () => {
		expect(renderMarkdown('## Summary\n---\nDone')).toBe('<p class="md-heading">Summary</p><p>Done</p>');
	});
});

describe('parseEvent', () => {
	it('reads a named event', () => {
		expect(parseEvent('event: text\ndata: {"text":"hi"}')).toEqual({ name: 'text', data: { text: 'hi' } });
	});

	it('ignores keep-alive comments', () => {
		expect(parseEvent(': ping')).toBeNull();
	});
});
