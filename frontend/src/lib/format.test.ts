import { describe, expect, it } from 'vitest';
import { formatCards, formatDayHeading, groupByDate } from './format';

describe('formatDayHeading', () => {
	it('names today and yesterday', () => {
		expect(formatDayHeading('2026-09-23', '2026-09-23')).toBe('Today');
		expect(formatDayHeading('2026-09-22', '2026-09-23')).toBe('Yesterday');
	});

	it('shows the weekday and leaves off this year', () => {
		expect(formatDayHeading('2026-09-20', '2026-09-23')).toBe('Sun, Sep 20');
	});

	it('includes the year for an earlier year', () => {
		expect(formatDayHeading('2025-12-31', '2026-01-02')).toBe('Wed, Dec 31, 2025');
	});

	it('crosses a DST change without an off-by-one', () => {
		expect(formatDayHeading('2026-11-01', '2026-11-02')).toBe('Yesterday');
	});
});

describe('groupByDate', () => {
	it('groups consecutive runs and keeps order', () => {
		const rows = [
			{ id: 1, date: '2026-09-23' },
			{ id: 2, date: '2026-09-23' },
			{ id: 3, date: '2026-09-21' }
		];
		expect(groupByDate(rows, (r) => r.date)).toEqual([
			{ date: '2026-09-23', items: [rows[0], rows[1]] },
			{ date: '2026-09-21', items: [rows[2]] }
		]);
	});

	it('returns nothing for nothing', () => {
		expect(groupByDate([], () => '')).toEqual([]);
	});
});

describe('formatCards', () => {
	it('shows each card masked', () => {
		expect(formatCards('1234')).toBe('····1234');
		expect(formatCards('1234, 5678')).toBe('····1234, ····5678');
		expect(formatCards(null)).toBe('');
	});
});
