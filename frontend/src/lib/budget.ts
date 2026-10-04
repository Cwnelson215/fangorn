import { today } from './format';
import type { Budget } from './types';

/**
 * How much of a budget is used. Only `over` and `committed` say anything under
 * the bar; `half` just changes its colour.
 *
 * - `over`: already past the limit.
 * - `committed`: under the limit, but recurring charges still to post this month
 *   would take it over.
 * - `half`: half the limit or more is spent.
 * - `ok`: less than half.
 *
 * None of it depends on the date: a big shop on the 2nd isn't a warning.
 */
export type PaceStatus = 'over' | 'committed' | 'half' | 'ok';

export interface BudgetPace {
	status: PaceStatus;
	/** Fraction of the month elapsed through today, or null when `month` isn't the current month. */
	elapsed: number | null;
}

/** Fraction of `month` (a YYYY-MM-01 date) that has passed, counting today as passed. */
export function monthElapsed(month: string, on: string = today()): number {
	const current = on.slice(0, 7) + '-01';
	if (month < current) return 1;
	if (month > current) return 0;
	const [y, m, d] = on.split('-').map(Number);
	const daysInMonth = new Date(y, m, 0).getDate();
	return d / daysInMonth;
}

export function budgetPace(
	{ spent, amount, scheduled }: Pick<Budget, 'spent' | 'amount' | 'scheduled'>,
	month: string,
	on: string = today()
): BudgetPace {
	const inProgress = month === on.slice(0, 7) + '-01';

	let status: PaceStatus = 'ok';
	if (spent > amount) {
		status = 'over';
	} else if (spent + scheduled > amount) {
		status = 'committed';
	} else if (amount > 0 && spent >= amount / 2) {
		status = 'half';
	}
	return { status, elapsed: inProgress ? monthElapsed(month, on) : null };
}

/**
 * Expected income has no "over": more arriving than planned is good news, and
 * less so far is usually a paycheck still to come. Its bar only marks how far
 * through the month it is.
 */
export function incomePace(month: string, on: string = today()): BudgetPace {
	const inProgress = month === on.slice(0, 7) + '-01';
	return { status: 'ok', elapsed: inProgress ? monthElapsed(month, on) : null };
}
