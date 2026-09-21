import { today } from './format';
import type { Budget } from './types';

/**
 * How a budget is tracking against the calendar.
 *
 * - `over`: already past the limit.
 * - `committed`: under the limit, but recurring charges still to post this month
 *   would take it over. Certain rather than projected, so it outranks `ahead`.
 * - `ahead`: under the limit, but spending faster than the month is passing, by
 *   more than PACE_TOLERANCE. Spending evenly would pass the limit.
 * - `ok`: everything else. Pace only means something while the month is in
 *   progress; `over` and `committed` apply to any month.
 */
export type PaceStatus = 'over' | 'committed' | 'ahead' | 'ok';

export interface BudgetPace {
	status: PaceStatus;
	/** Fraction of the month elapsed through today, or null when `month` isn't the current month. */
	elapsed: number | null;
	/** Month-end spend if the rest of the month goes like the part so far; null outside the current month. */
	projected: number | null;
}

/**
 * Ten percentage points of slack, so a single big grocery run on the 2nd doesn't
 * flag the whole month. Spending 60% of a limit by the halfway point is fine;
 * 61% is ahead.
 */
const PACE_TOLERANCE = 0.1;

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
	const elapsed = inProgress ? monthElapsed(month, on) : null;
	const projected = elapsed ? spent / elapsed : null;

	let status: PaceStatus = 'ok';
	if (spent > amount) {
		status = 'over';
	} else if (spent + scheduled > amount) {
		status = 'committed';
	} else if (elapsed !== null && amount > 0 && spent / amount > elapsed + PACE_TOLERANCE) {
		status = 'ahead';
	}
	return { status, elapsed, projected };
}
