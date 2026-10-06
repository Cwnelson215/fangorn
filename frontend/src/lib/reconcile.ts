// Reconciling an investment account against the brokerage's own figures: which
// line is off, by how much, and why. Pure — the form in ReconcileModal feeds it.
//
// A position's value can differ for two reasons that need different fixes. A
// different share count is a missing or mistyped trade (or a reinvested
// dividend never logged); the same shares at a different value is only the
// price, which the next quote or the market close settles by itself. Cash
// differs when a dividend, fee or trade amount is missing or off.

export interface OurPosition {
	symbol: string;
	shares: number;
	price: number;
	market_value: number;
}

/** What the statement shows for one line; null = not entered. */
export interface StatementLine {
	shares: number | null;
	value: number | null;
}

export type Verdict = 'match' | 'shares' | 'price' | 'unchecked';

export interface PositionCheck {
	symbol: string;
	verdict: Verdict;
	/** Statement shares − ours; 0 when no share count was entered. */
	shareDiff: number;
	/** Statement value − ours; 0 when no value was entered. */
	valueDiff: number;
	/** The part of valueDiff the share difference explains, at our price. */
	fromShares: number;
	/** The rest: the same shares priced differently. */
	fromPrice: number;
}

export interface Reconciliation {
	positions: PositionCheck[];
	/** Statement cash − ours; null when cash wasn't entered. */
	cashDiff: number | null;
	/** Sum over everything entered: statement − ours. */
	totalDiff: number;
	/** The parts of totalDiff, by cause. */
	fromCash: number;
	fromShares: number;
	fromPrice: number;
	/** True once something was entered and every entered line matches. */
	matches: boolean;
	checked: number;
}

// Brokerages show shares to three decimals and money to the cent.
const SHARE_TOLERANCE = 0.0005;
const CENT = 0.005;

function cents(v: number): number {
	return Math.round(v * 100) / 100;
}

export function reconcile(
	ours: { cash: number; positions: OurPosition[] },
	statement: { cash: number | null; lines: Record<string, StatementLine | undefined> }
): Reconciliation {
	let checked = 0;
	const positions = ours.positions.map((p): PositionCheck => {
		const line = statement.lines[p.symbol];
		const shareDiff = line?.shares != null ? line.shares - p.shares : 0;
		const sharesOff = Math.abs(shareDiff) > SHARE_TOLERANCE;
		if (line?.value == null) {
			// A share count alone still tells whether a trade is missing.
			const fromShares = sharesOff ? cents(shareDiff * p.price) : 0;
			if (line?.shares != null) checked++;
			return {
				symbol: p.symbol,
				verdict: sharesOff ? 'shares' : line?.shares != null ? 'match' : 'unchecked',
				shareDiff: sharesOff ? shareDiff : 0,
				valueDiff: fromShares,
				fromShares,
				fromPrice: 0
			};
		}
		checked++;
		const valueDiff = cents(line.value - p.market_value);
		const fromShares = sharesOff ? cents(shareDiff * p.price) : 0;
		const fromPrice = cents(valueDiff - fromShares);
		const verdict: Verdict = sharesOff ? 'shares' : Math.abs(valueDiff) > CENT ? 'price' : 'match';
		return {
			symbol: p.symbol,
			verdict,
			shareDiff: sharesOff ? shareDiff : 0,
			valueDiff,
			fromShares,
			fromPrice
		};
	});

	const cashDiff = statement.cash != null ? cents(statement.cash - ours.cash) : null;
	if (cashDiff != null) checked++;
	const fromCash = cashDiff ?? 0;
	const fromShares = cents(positions.reduce((sum, p) => sum + p.fromShares, 0));
	const fromPrice = cents(positions.reduce((sum, p) => sum + p.fromPrice, 0));
	return {
		positions,
		cashDiff,
		totalDiff: cents(fromCash + fromShares + fromPrice),
		fromCash,
		fromShares,
		fromPrice,
		matches:
			checked > 0 &&
			Math.abs(fromCash) <= CENT &&
			positions.every((p) => p.verdict === 'match' || p.verdict === 'unchecked'),
		checked
	};
}
