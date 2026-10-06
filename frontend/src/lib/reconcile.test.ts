import { describe, expect, it } from 'vitest';
import { reconcile } from './reconcile';

const ours = {
	cash: 1071.07,
	positions: [
		{ symbol: 'AMD', shares: 1, price: 647.785, market_value: 647.78 },
		{ symbol: 'TOST', shares: 6, price: 30.2, market_value: 181.2 },
		{ symbol: 'FZROX', shares: 100, price: 27.08, market_value: 2708 }
	]
};

describe('reconcile', () => {
	it('checks nothing until something is entered', () => {
		const r = reconcile(ours, { cash: null, lines: {} });
		expect(r.checked).toBe(0);
		expect(r.matches).toBe(false);
		expect(r.positions.every((p) => p.verdict === 'unchecked')).toBe(true);
	});

	it('puts a missing dividend in cash', () => {
		const r = reconcile(ours, { cash: 1071.6, lines: {} });
		expect(r.cashDiff).toBe(0.53);
		expect(r.fromCash).toBe(0.53);
		expect(r.totalDiff).toBe(0.53);
		expect(r.matches).toBe(false);
	});

	it('calls the same shares at another value a price difference', () => {
		const r = reconcile(ours, {
			cash: 1071.07,
			lines: { AMD: { shares: 1, value: 647.67 }, TOST: { shares: 6, value: 181.29 } }
		});
		expect(r.positions.map((p) => p.verdict)).toEqual(['price', 'price', 'unchecked']);
		expect(r.fromPrice).toBe(-0.02);
		expect(r.fromShares).toBe(0);
		expect(r.totalDiff).toBe(-0.02);
	});

	it('splits a value gap between a missing trade and the price', () => {
		// The statement has 1.2 more shares (a reinvested dividend) and a penny
		// of price movement on top.
		const r = reconcile(ours, { cash: null, lines: { FZROX: { shares: 101.2, value: 2741.51 } } });
		const fzrox = r.positions[2];
		expect(fzrox.verdict).toBe('shares');
		expect(fzrox.shareDiff).toBeCloseTo(1.2);
		expect(fzrox.fromShares).toBe(32.5);
		expect(fzrox.fromPrice).toBe(1.01);
		expect(r.totalDiff).toBe(33.51);
	});

	it('flags a share count entered without a value', () => {
		const r = reconcile(ours, { cash: null, lines: { TOST: { shares: 7, value: null } } });
		expect(r.positions[1].verdict).toBe('shares');
		expect(r.positions[1].fromShares).toBe(30.2);
	});

	it('matches within a cent and the three decimals a statement shows', () => {
		const r = reconcile(ours, {
			cash: 1071.07,
			lines: {
				AMD: { shares: 1, value: 647.78 },
				TOST: { shares: 6.0004, value: 181.2 },
				FZROX: { shares: 100, value: 2708 }
			}
		});
		expect(r.matches).toBe(true);
		expect(r.totalDiff).toBe(0);
	});
});
