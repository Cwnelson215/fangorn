import { describe, expect, it } from 'vitest';
import type { RecurringRule } from './types';
import { addMonths, formatDuration, monthlyEquivalent, monthlyInflow, projectGrowth, projectPayoff } from './projection';

describe('projectGrowth', () => {
	it('compounds a yearly return monthly to exactly that return over a year', () => {
		const pts = projectGrowth({ start: 1000, monthly: 0, annualReturn: 0.07, months: 12 });
		expect(pts).toHaveLength(13);
		expect(pts[0].balance).toBe(1000);
		expect(pts[12].balance).toBeCloseTo(1070, 2);
		expect(pts[12].contributed).toBe(1000);
		expect(pts[12].growth).toBeCloseTo(70, 2);
	});

	it('adds contributions at the end of each month', () => {
		const pts = projectGrowth({ start: 0, monthly: 100, annualReturn: 0, months: 24 });
		expect(pts[24].balance).toBe(2400);
		expect(pts[24].contributed).toBe(2400);
		expect(pts[24].growth).toBe(0);
	});

	it('steps contributions up once a year', () => {
		const pts = projectGrowth({ start: 0, monthly: 100, annualReturn: 0, months: 24, raise: 0.1 });
		expect(pts[12].contributed).toBe(1200);
		expect(pts[24].contributed).toBeCloseTo(1200 + 1320, 2);
	});

	it('applies one-time deposits and withdrawals in their month', () => {
		const pts = projectGrowth({
			start: 1000,
			monthly: 0,
			annualReturn: 0,
			months: 12,
			oneTime: [
				{ month: 3, amount: 500 },
				{ month: 6, amount: -200 }
			]
		});
		expect(pts[2].balance).toBe(1000);
		expect(pts[3].balance).toBe(1500);
		expect(pts[12].balance).toBe(1300);
		expect(pts[12].contributed).toBe(1300);
	});

	it('deflates into today’s dollars', () => {
		const pts = projectGrowth({ start: 1000, monthly: 0, annualReturn: 0.03, months: 12, inflation: 0.03 });
		expect(pts[12].balance).toBeCloseTo(1000, 2);
	});
});

describe('projectPayoff', () => {
	it('pays off a small balance and counts the interest', () => {
		const r = projectPayoff({ owed: 1000, apr: 0.12, payment: 500 });
		// 1000 → +10 interest → 510; → +5.10 → 15.10; → +0.15 → 0.
		expect(r.months).toBe(3);
		expect(r.totalInterest).toBeCloseTo(15.25, 2);
		expect(r.points[r.points.length - 1].owed).toBe(0);
		expect(r.points[r.points.length - 1].paid).toBeCloseTo(1015.25, 2);
	});

	it('never pays off when the payment only covers interest', () => {
		const r = projectPayoff({ owed: 10_000, apr: 0.24, payment: 200 });
		expect(r.months).toBeNull();
		expect(r.points).toHaveLength(121);
		expect(r.points[120].owed).toBe(10_000);
	});

	it('is already paid off when nothing is owed', () => {
		expect(projectPayoff({ owed: 0, apr: 0.2, payment: 50 }).months).toBe(0);
	});

	it('matches a standard 30-year mortgage payment', () => {
		// $200k at 6% over 360 months is $1,199.10/month, rounded up to the cent
		// so the last payment clears it.
		const r = projectPayoff({ owed: 200_000, apr: 0.06, payment: 1199.11 });
		expect(r.months).toBe(360);
	});
});

const rule = (over: Partial<RecurringRule>): RecurringRule =>
	({
		id: 1,
		kind: 'transfer',
		account_id: 1,
		to_account_id: 2,
		amount: 100,
		frequency: 'monthly',
		interval_count: 1,
		paused: false,
		...over
	}) as RecurringRule;

describe('monthlyInflow', () => {
	it('converts frequencies to a monthly figure', () => {
		expect(monthlyEquivalent(rule({ frequency: 'biweekly' }))).toBeCloseTo(216.67, 2);
		expect(monthlyEquivalent(rule({ frequency: 'monthly', interval_count: 2 }))).toBe(50);
		expect(monthlyEquivalent(rule({ frequency: 'yearly', amount: 1200 }))).toBe(100);
	});

	it('counts transfers in and income, not transfers out or paused rules', () => {
		const rules = [
			rule({ to_account_id: 2, amount: 500 }),
			rule({ account_id: 2, to_account_id: 3, amount: 50 }),
			rule({ kind: 'income', account_id: 2, to_account_id: null, amount: 20 }),
			rule({ to_account_id: 2, amount: 999, paused: true })
		];
		expect(monthlyInflow(rules, 2)).toBe(520);
	});
});

describe('dates', () => {
	it('adds months, clamping to the month end', () => {
		expect(addMonths('2026-01-31', 1)).toBe('2026-02-28');
		expect(addMonths('2026-09-24', 15)).toBe('2027-12-24');
	});

	it('formats durations', () => {
		expect(formatDuration(40)).toBe('3 yr 4 mo');
		expect(formatDuration(11)).toBe('11 mo');
		expect(formatDuration(24)).toBe('2 yr');
	});
});
