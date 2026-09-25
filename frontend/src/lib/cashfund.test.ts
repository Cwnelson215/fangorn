import { describe, expect, it } from 'vitest';
import { defaultCashFund } from './cashfund';

describe('defaultCashFund', () => {
	it('knows the brokerages whose cash sits in a money market fund', () => {
		expect(defaultCashFund('Fidelity')).toBe('SPAXX');
		expect(defaultCashFund('fidelity investments')).toBe('SPAXX');
		expect(defaultCashFund('Vanguard')).toBe('VMFXX');
	});

	it('guesses nothing for anyone else', () => {
		expect(defaultCashFund('Gesa')).toBeNull();
		expect(defaultCashFund('')).toBeNull();
		expect(defaultCashFund(null)).toBeNull();
	});
});
