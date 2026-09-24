// Shared formatters. Before the pivot, Intl.NumberFormat was reconstructed in six
// separate components; keeping one instance here also avoids rebuilding the
// formatter on every render.

const currency = new Intl.NumberFormat('en-US', {
	style: 'currency',
	currency: 'USD'
});

const currencyWhole = new Intl.NumberFormat('en-US', {
	style: 'currency',
	currency: 'USD',
	maximumFractionDigits: 0
});

export function formatCurrency(amount: number): string {
	return currency.format(amount);
}

export function formatCurrencyWhole(amount: number): string {
	return currencyWhole.format(amount);
}

/**
 * Formats a signed amount with an explicit + or −, for transaction rows where
 * the direction matters as much as the size.
 */
export function formatSigned(amount: number): string {
	const sign = amount < 0 ? '−' : '+';
	return sign + currency.format(Math.abs(amount));
}

/** A price, which unlike an amount can carry up to four decimals: $19.905. */
const price = new Intl.NumberFormat('en-US', {
	style: 'currency',
	currency: 'USD',
	minimumFractionDigits: 2,
	maximumFractionDigits: 4
});

export function formatPrice(amount: number): string {
	return price.format(amount);
}

const shares = new Intl.NumberFormat('en-US', { maximumFractionDigits: 4 });

/** Share counts without trailing zeros: 12.5, 25.123, 100. */
export function formatShares(n: number): string {
	return shares.format(n);
}

const percent = new Intl.NumberFormat('en-US', {
	style: 'percent',
	minimumFractionDigits: 2,
	maximumFractionDigits: 2
});

/** Formats a ratio (0.0123 → "1.23%"). Signed adds + or − like formatSigned. */
export function formatPercent(ratio: number, signed = false): string {
	if (!signed) return percent.format(ratio);
	const sign = ratio < 0 ? '−' : '+';
	return sign + percent.format(Math.abs(ratio));
}

/** "4:00 PM ET" today, or "Sep 11, 4:00 PM ET" for an older price. */
export function formatMarketTime(iso: string): string {
	const d = new Date(iso);
	const sameDay =
		d.toLocaleDateString('en-US', { timeZone: 'America/New_York' }) ===
		new Date().toLocaleDateString('en-US', { timeZone: 'America/New_York' });
	const time = d.toLocaleTimeString('en-US', {
		timeZone: 'America/New_York',
		hour: 'numeric',
		minute: '2-digit'
	});
	if (sameDay) return `${time} ET`;
	const day = d.toLocaleDateString('en-US', {
		timeZone: 'America/New_York',
		month: 'short',
		day: 'numeric'
	});
	return `${day}, ${time} ET`;
}

/**
 * Parses a YYYY-MM-DD date without timezone drift. `new Date('2026-01-15')`
 * parses as UTC midnight and renders as the 14th anywhere west of Greenwich, so
 * the time component is appended to force local interpretation.
 */
export function parseDate(date: string): Date {
	return new Date(date + 'T00:00:00');
}

export function formatDate(date: string): string {
	return parseDate(date).toLocaleDateString('en-US', {
		month: 'short',
		day: 'numeric',
		year: 'numeric'
	});
}

export function formatDateShort(date: string): string {
	return parseDate(date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

export function formatMonth(date: string): string {
	return parseDate(date).toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
}

/** Today as YYYY-MM-DD in the browser's local timezone. */
export function today(): string {
	const now = new Date();
	const local = new Date(now.getTime() - now.getTimezoneOffset() * 60000);
	return local.toISOString().slice(0, 10);
}

/** The first of the current month, as YYYY-MM-DD. */
export function monthStart(): string {
	return today().slice(0, 7) + '-01';
}

/** Moves a YYYY-MM-DD month start by `delta` months. */
export function shiftMonth(month: string, delta: number): string {
	const [y, m] = month.split('-').map(Number);
	const d = new Date(y, m - 1 + delta, 1);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`;
}

/** Describes how far away a date is, e.g. "in 3 days" or "2 days ago". */
export function relativeDays(date: string): string {
	const target = parseDate(date);
	const now = parseDate(today());
	const days = Math.round((target.getTime() - now.getTime()) / 86400000);

	if (days === 0) return 'today';
	if (days === 1) return 'tomorrow';
	if (days === -1) return 'yesterday';
	if (days > 0) return `in ${days} days`;
	return `${Math.abs(days)} days ago`;
}

/**
 * Heading for a day in a phone-sized list: "Today", "Yesterday", "Mon, Sep 22",
 * with the year only once it isn't this year's.
 */
export function formatDayHeading(date: string, now: string = today()): string {
	const days = Math.round((parseDate(now).getTime() - parseDate(date).getTime()) / 86400000);
	if (days === 0) return 'Today';
	if (days === 1) return 'Yesterday';
	const d = parseDate(date);
	return d.toLocaleDateString('en-US', {
		weekday: 'short',
		month: 'short',
		day: 'numeric',
		year: d.getFullYear() === parseDate(now).getFullYear() ? undefined : 'numeric'
	});
}

/**
 * Splits a newest-first list into runs of the same date, keeping order. Rows
 * only carry the date on desktop; on a phone each run gets a heading instead.
 */
export function groupByDate<T>(items: T[], dateOf: (item: T) => string): { date: string; items: T[] }[] {
	const groups: { date: string; items: T[] }[] = [];
	for (const item of items) {
		const date = dateOf(item);
		const last = groups[groups.length - 1];
		if (last && last.date === date) last.items.push(item);
		else groups.push({ date, items: [item] });
	}
	return groups;
}
