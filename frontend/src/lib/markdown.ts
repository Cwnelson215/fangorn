// Renders the assistant's answers: the small slice of markdown it is asked to
// write (paragraphs, bullet and numbered lists, small tables, bold, italics,
// inline code, and the odd heading) and nothing else.
//
// Everything is HTML-escaped before any markup is added, so text from the model
// — which can quote descriptions and notes the family typed — can never become
// markup. Links and images are deliberately not supported: nothing the
// assistant says should be something to click.

const ESCAPES: Record<string, string> = {
	'&': '&amp;',
	'<': '&lt;',
	'>': '&gt;',
	'"': '&quot;',
	"'": '&#39;'
};

export function escapeHtml(s: string): string {
	return s.replace(/[&<>"']/g, (c) => ESCAPES[c]);
}

/** Inline markup on already-escaped text. Code spans are protected first. */
function inline(escaped: string): string {
	const code: string[] = [];
	let s = escaped.replace(/`([^`]+)`/g, (_, c: string) => {
		code.push(c);
		return `\u0000${code.length - 1}\u0000`;
	});
	s = s
		.replace(/\*\*(?=\S)([\s\S]*?\S)\*\*/g, '<strong>$1</strong>')
		// Single-star italics only: underscores turn up in names and ids.
		.replace(/(^|[^*\w])\*(?=\S)([^*]*?\S)\*(?!\*)/g, '$1<em>$2</em>');
	return s.replace(/\u0000(\d+)\u0000/g, (_, i: string) => `<code>${code[Number(i)]}</code>`);
}

const BULLET = /^\s*[-*•]\s+(.*)$/;
const ORDERED = /^\s*\d+[.)]\s+(.*)$/;
const HEADING = /^(#{1,6})\s+(.*)$/;
const RULE = /^\s*([-*_])(\s*\1){2,}\s*$/;
const TABLE_ROW = /^\s*\|.*\|\s*$/;
const TABLE_DIVIDER = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;

function cells(row: string): string[] {
	return row
		.trim()
		.replace(/^\|/, '')
		.replace(/\|$/, '')
		.split('|')
		.map((c) => c.trim());
}

/** Right-aligns a column whose cells all look like amounts or counts. */
function numericColumns(rows: string[][]): boolean[] {
	const width = Math.max(0, ...rows.map((r) => r.length));
	return Array.from({ length: width }, (_, i) =>
		rows.every((r) => !r[i] || /^[−\-+]?\(?\$?[\d,.]+%?\)?$/.test(r[i].replace(/\*\*/g, '')))
	);
}

function table(lines: string[]): string {
	const head = cells(lines[0]);
	const body = lines.slice(2).map(cells);
	const numeric = numericColumns(body);
	const cell = (tag: string, c: string, i: number) =>
		`<${tag}${numeric[i] ? ' class="num"' : ''}>${inline(escapeHtml(c))}</${tag}>`;
	return (
		'<div class="md-table"><table><thead><tr>' +
		head.map((c, i) => cell('th', c, i)).join('') +
		'</tr></thead><tbody>' +
		body.map((r) => '<tr>' + r.map((c, i) => cell('td', c, i)).join('') + '</tr>').join('') +
		'</tbody></table></div>'
	);
}

export function renderMarkdown(text: string): string {
	const lines = text.replace(/\r\n?/g, '\n').split('\n');
	const out: string[] = [];
	let i = 0;

	while (i < lines.length) {
		const line = lines[i];

		if (line.trim() === '' || RULE.test(line)) {
			i++;
			continue;
		}

		const heading = HEADING.exec(line);
		if (heading) {
			// Headings are kept small: they sit inside a chat bubble.
			out.push(`<p class="md-heading">${inline(escapeHtml(heading[2]))}</p>`);
			i++;
			continue;
		}

		if (TABLE_ROW.test(line) && i + 1 < lines.length && TABLE_DIVIDER.test(lines[i + 1])) {
			const rows = [line, lines[i + 1]];
			i += 2;
			while (i < lines.length && TABLE_ROW.test(lines[i])) rows.push(lines[i++]);
			out.push(table(rows));
			continue;
		}

		const list = BULLET.test(line) ? BULLET : ORDERED.test(line) ? ORDERED : null;
		if (list) {
			const items: string[] = [];
			while (i < lines.length) {
				const m = list.exec(lines[i]);
				if (m) {
					items.push(m[1]);
				} else if (lines[i].trim() !== '' && /^\s+/.test(lines[i]) && items.length) {
					// An indented continuation of the item above.
					items[items.length - 1] += ' ' + lines[i].trim();
				} else {
					break;
				}
				i++;
			}
			const tag = list === BULLET ? 'ul' : 'ol';
			out.push(`<${tag}>` + items.map((it) => `<li>${inline(escapeHtml(it))}</li>`).join('') + `</${tag}>`);
			continue;
		}

		const para: string[] = [];
		while (
			i < lines.length &&
			lines[i].trim() !== '' &&
			!HEADING.test(lines[i]) &&
			!BULLET.test(lines[i]) &&
			!ORDERED.test(lines[i]) &&
			!(TABLE_ROW.test(lines[i]) && TABLE_DIVIDER.test(lines[i + 1] ?? ''))
		) {
			para.push(lines[i++]);
		}
		out.push('<p>' + para.map((l) => inline(escapeHtml(l.trim()))).join('<br>') + '</p>');
	}
	return out.join('');
}
