import { describe, it, expect } from 'vitest';
import { budgetPace, incomePace, monthElapsed } from './budget';

/**
 * These functions decide what colour a budget bar is, what warning sits under
 * it and where its "today" mark goes. Every test here pins `on` explicitly: a
 * test that reads the clock would pass all month and fail on the 31st.
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
		expect(budgetPace(budget(450, 400), '2026-03-01', '2026-03-10').status).toBe('over');
	});

	it('flags a budget that scheduled charges will take over', () => {
		// $300 spent of $400 is under the limit; the $150 subscription still to
		// post this month is what makes it certain to bust.
		expect(budgetPace(budget(300, 400, 150), '2026-03-01', '2026-03-10').status).toBe('committed');
	});

	it('reports over rather than committed once the limit is actually passed', () => {
		// Both conditions hold here; `over` is the one that has already happened.
		expect(budgetPace(budget(450, 400, 150), '2026-03-01', '2026-03-10').status).toBe('over');
	});

	it('turns at half the limit, and stays there up to the limit itself', () => {
		expect(budgetPace(budget(199.99, 400), '2026-03-01', '2026-03-16').status).toBe('ok');
		expect(budgetPace(budget(200, 400), '2026-03-01', '2026-03-16').status).toBe('half');
		// Spending exactly the limit isn't over it.
		expect(budgetPace(budget(400, 400), '2026-03-01', '2026-03-16').status).toBe('half');
	});

	it('does not care how far through the month it is', () => {
		// 75% spent on the 2nd used to be "ahead of pace"; now it is the same as
		// 75% spent on the 30th, or in a month already over.
		for (const on of ['2026-03-02', '2026-03-30', '2026-05-15']) {
			expect(budgetPace(budget(300, 400), '2026-03-01', on).status).toBe('half');
		}
		expect(budgetPace(budget(100, 400), '2026-03-01', '2026-03-02').status).toBe('ok');
	});

	it('marks the month only while it is in progress', () => {
		expect(budgetPace(budget(0, 400), '2026-03-01', '2026-03-16').elapsed).toBeCloseTo(16 / 31);
		expect(budgetPace(budget(0, 400), '2026-01-01', '2026-03-15').elapsed).toBeNull();
		expect(budgetPace(budget(0, 400), '2026-06-01', '2026-03-15').elapsed).toBeNull();
	});

	it('still reports over and committed outside the current month', () => {
		expect(budgetPace(budget(450, 400), '2026-01-01', '2026-03-15').status).toBe('over');
		expect(budgetPace(budget(0, 400, 500), '2026-06-01', '2026-03-15').status).toBe('committed');
	});

	it('calls a zero budget with nothing spent ok', () => {
		// SetBudget rejects a zero amount, but one must not read as half used.
		expect(budgetPace(budget(0, 0), '2026-03-01', '2026-03-16').status).toBe('ok');
	});
});

describe('incomePace', () => {
	it('is never over and marks the month only while it is in progress', () => {
		expect(incomePace('2026-09-01', '2026-09-15')).toEqual({ status: 'ok', elapsed: 0.5 });
		expect(incomePace('2026-08-01', '2026-09-15').elapsed).toBeNull();
	});
});
