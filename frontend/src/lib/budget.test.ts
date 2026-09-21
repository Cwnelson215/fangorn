import { describe, it, expect } from 'vitest';
import { budgetPace, monthElapsed } from './budget';

/**
 * These two functions decide what colour a budget bar is and what warning sits
 * under it, which makes them the most user-visible arithmetic in the app — and
 * the easiest to get subtly wrong, because every case is a date boundary. Every
 * test here pins `on` explicitly: a test that reads the clock would pass all
 * month and fail on the 31st.
 */

/** The shape budgetPace destructures, so cases read as money rather than args. */
const budget = (spent: number, amount: number, scheduled = 0) => ({ spent, amount, scheduled });

describe('monthElapsed', () => {
	it('is 1 for a month already over and 0 for one not started', () => {
		// A finished month is fully elapsed no matter which day you ask on, and a
		// future month hasn't begun — pace is meaningless in both.
		expect(monthElapsed('2026-01-01', '2026-03-15')).toBe(1);
		expect(monthElapsed('2026-06-01', '2026-03-15')).toBe(0);
	});

	it('counts today as a day that has passed', () => {
		// The 15th of a 31-day month means 15 days are gone, not 14: someone who
		// has spent all day spending should be measured against the whole day.
		expect(monthElapsed('2026-03-01', '2026-03-15')).toBeCloseTo(15 / 31);
		expect(monthElapsed('2026-03-01', '2026-03-01')).toBeCloseTo(1 / 31);
		expect(monthElapsed('2026-03-01', '2026-03-31')).toBe(1);
	});

	it('uses the real length of short months', () => {
		// February is the month where a hardcoded 30 or 31 would show up as a bar
		// that never quite reaches the end of the month.
		expect(monthElapsed('2026-02-01', '2026-02-28')).toBe(1);
		expect(monthElapsed('2026-02-01', '2026-02-14')).toBeCloseTo(14 / 28);
		// 2028 is a leap year, so the 29th exists and is the whole month.
		expect(monthElapsed('2028-02-01', '2028-02-29')).toBe(1);
		expect(monthElapsed('2028-02-01', '2028-02-14')).toBeCloseTo(14 / 29);
	});
});

describe('budgetPace', () => {
	it('flags a budget that is already over', () => {
		const pace = budgetPace(budget(450, 400), '2026-03-01', '2026-03-10');
		expect(pace.status).toBe('over');
	});

	it('flags a budget that scheduled charges will take over', () => {
		// $300 spent of $400 is fine on its own; the $150 subscription still to
		// post this month is what makes it certain to bust. That is a fact, not a
		// projection, so it must not be reported as mere pace.
		expect(budgetPace(budget(300, 400, 150), '2026-03-01', '2026-03-10').status).toBe('committed');
		// Without the commitment the same spend is simply fine.
		expect(budgetPace(budget(300, 400), '2026-03-01', '2026-03-02').status).toBe('ahead');
	});

	it('reports over rather than committed once the limit is actually passed', () => {
		// Both conditions hold here; `over` is the one that has already happened.
		expect(budgetPace(budget(450, 400, 150), '2026-03-01', '2026-03-10').status).toBe('over');
	});

	it('flags spending that outruns the calendar', () => {
		// Half the month gone, 75% of the limit spent: on this trajectory the month
		// ends at $600 against a $400 budget.
		const pace = budgetPace(budget(300, 400), '2026-03-01', '2026-03-16');
		expect(pace.status).toBe('ahead');
		expect(pace.projected).toBeCloseTo(300 / (16 / 31));
	});

	it('allows ten points of slack before calling it ahead', () => {
		// One big shop early in the month shouldn't light up the whole page, so the
		// threshold is elapsed + 0.1, and it is strictly greater — landing exactly
		// on the line is still fine. Against a $1 budget the spend *is* the
		// fraction, which keeps the boundary exact instead of nearly-exact.
		const elapsed = 16 / 31; // asking on the 16th of a 31-day month
		expect(budgetPace(budget(elapsed + 0.1, 1), '2026-03-01', '2026-03-16').status).toBe('ok');
		expect(budgetPace(budget(elapsed + 0.101, 1), '2026-03-01', '2026-03-16').status).toBe(
			'ahead'
		);
	});

	it('says nothing about pace outside the current month', () => {
		// A finished month can't be "ahead of pace" — it's simply what happened —
		// and a future month hasn't had the chance.
		const past = budgetPace(budget(390, 400), '2026-01-01', '2026-03-15');
		expect(past.status).toBe('ok');
		expect(past.elapsed).toBeNull();
		expect(past.projected).toBeNull();

		const future = budgetPace(budget(0, 400), '2026-06-01', '2026-03-15');
		expect(future.status).toBe('ok');
		expect(future.projected).toBeNull();
	});

	it('still reports over and committed outside the current month', () => {
		// Those two are facts about the money, not about the calendar.
		expect(budgetPace(budget(450, 400), '2026-01-01', '2026-03-15').status).toBe('over');
		expect(budgetPace(budget(0, 400, 500), '2026-06-01', '2026-03-15').status).toBe('committed');
	});

	it('does not divide by a zero budget', () => {
		// SetBudget rejects a zero amount, but the bar must not render NaN if one
		// ever reaches it.
		const pace = budgetPace(budget(0, 0), '2026-03-01', '2026-03-16');
		expect(pace.status).toBe('ok');
		expect(Number.isNaN(pace.projected ?? 0)).toBe(false);
	});
});
