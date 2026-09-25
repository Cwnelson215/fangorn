// Projections for the account pages: what a balance grows to, or how long a
// debt takes to pay off, under assumptions the user drags around. Pure — no
// dates beyond month counting, no fetching — so the charts and the numbers
// beside them come from the same place.

import type { Frequency, RecurringRule } from './types';

const round2 = (n: number) => Math.round(n * 100) / 100;

export interface GrowthInput {
	/** Today's balance. */
	start: number;
	/** Added at the end of every month. */
	monthly: number;
	/** Expected yearly return as a ratio (0.07 = 7%), compounded monthly like an APY. */
	annualReturn: number;
	months: number;
	/**
	 * Yearly inflation as a ratio. When set, every figure is deflated into
	 * today's dollars — what the future balance would buy now.
	 */
	inflation?: number;
}

export interface GrowthPoint {
	/** Months from now; 0 is today. */
	month: number;
	balance: number;
	/** The starting balance plus every contribution so far. */
	contributed: number;
	/** What the money earned on top: balance − contributed. */
	growth: number;
}

/**
 * Month-by-month growth of a balance with a fixed monthly contribution. The
 * yearly return is treated as an effective annual rate, so 12 months at 7% with
 * no contributions lands on exactly 1.07× — the way an APY or a fund's annual
 * return is quoted.
 */
export function projectGrowth({ start, monthly, annualReturn, months, inflation = 0 }: GrowthInput): GrowthPoint[] {
	const rate = Math.pow(1 + annualReturn, 1 / 12) - 1;
	const deflate = Math.pow(1 + inflation, 1 / 12);
	const out: GrowthPoint[] = [];
	let balance = start;
	let contributed = start;
	for (let m = 0; m <= Math.max(0, Math.floor(months)); m++) {
		if (m > 0) {
			balance = balance * (1 + rate) + monthly;
			contributed += monthly;
		}
		const d = Math.pow(deflate, m);
		out.push({
			month: m,
			balance: round2(balance / d),
			contributed: round2(contributed / d),
			growth: round2((balance - contributed) / d)
		});
	}
	return out;
}

export interface PayoffInput {
	/** What is owed today, as a positive number. */
	owed: number;
	/** Annual percentage rate as a ratio (0.22 = 22%), charged monthly at apr ÷ 12. */
	apr: number;
	/** Paid at the end of every month, after that month's interest. */
	payment: number;
	/** Stop after this many months whether or not it's paid off. */
	maxMonths?: number;
}

export interface PayoffPoint {
	month: number;
	owed: number;
	/** Interest charged so far. */
	interest: number;
	/** Payments so far. */
	paid: number;
}

export interface PayoffResult {
	points: PayoffPoint[];
	/** Months until nothing is owed; null if the payment never gets there. */
	months: number | null;
	/** Interest paid over the whole payoff (or over maxMonths if it never ends). */
	totalInterest: number;
}

/**
 * Month-by-month payoff of a debt at a fixed payment. A payment that doesn't
 * cover the first month's interest never pays it down, and the series shows
 * the balance growing instead.
 */
export function projectPayoff({ owed, apr, payment, maxMonths = 600 }: PayoffInput): PayoffResult {
	let balance = Math.max(0, owed);
	let interest = 0;
	let paid = 0;
	const points: PayoffPoint[] = [{ month: 0, owed: round2(balance), interest: 0, paid: 0 }];
	if (balance < 0.005) return { points, months: 0, totalInterest: 0 };

	const firstInterest = round2(balance * (apr / 12));
	// A payment at or below the first month's interest can only tread water;
	// show ten years of it rather than fifty.
	const limit = payment <= firstInterest ? Math.min(maxMonths, 120) : maxMonths;

	for (let m = 1; m <= limit; m++) {
		const charge = round2(balance * (apr / 12));
		balance = round2(balance + charge);
		interest = round2(interest + charge);
		const pay = Math.min(Math.max(0, payment), balance);
		balance = round2(balance - pay);
		paid = round2(paid + pay);
		points.push({ month: m, owed: balance, interest, paid });
		if (balance < 0.005) return { points, months: m, totalInterest: interest };
	}
	return { points, months: null, totalInterest: interest };
}

const PER_MONTH: Record<Frequency, number> = {
	daily: 365 / 12,
	weekly: 52 / 12,
	biweekly: 26 / 12,
	semimonthly: 2,
	monthly: 1,
	quarterly: 1 / 3,
	yearly: 1 / 12
};

/** A recurring amount expressed per month: $100 every 2 weeks ≈ $216.67. */
export function monthlyEquivalent(rule: Pick<RecurringRule, 'amount' | 'frequency' | 'interval_count'>): number {
	return (Math.abs(rule.amount) * PER_MONTH[rule.frequency]) / Math.max(1, rule.interval_count);
}

/**
 * How much the household's own recurring rules put into an account each month:
 * scheduled transfers in (a paycheck contribution, a card payment) and income
 * landing there. The projection cards start from this, so "what if I keep
 * doing what I'm doing" is the first answer they give.
 */
export function monthlyInflow(rules: RecurringRule[], accountId: number): number {
	let total = 0;
	for (const r of rules) {
		if (r.paused) continue;
		const lands =
			(r.kind === 'transfer' && r.to_account_id === accountId) ||
			(r.kind === 'income' && r.account_id === accountId);
		if (lands) total += monthlyEquivalent(r);
	}
	return round2(total);
}

/** Adds whole months to a YYYY-MM-DD date, clamping to the month's last day. */
export function addMonths(date: string, months: number): string {
	const [y, m, d] = date.split('-').map(Number);
	const first = new Date(y, m - 1 + months, 1);
	const last = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate();
	const day = Math.min(d, last);
	return `${first.getFullYear()}-${String(first.getMonth() + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

/** "3 yr 4 mo", "11 mo", "2 yr". */
export function formatDuration(months: number): string {
	const y = Math.floor(months / 12);
	const m = months % 12;
	if (y === 0) return `${m} mo`;
	if (m === 0) return `${y} yr`;
	return `${y} yr ${m} mo`;
}
