import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
	defaultAccount,
	pickRemembered,
	recallId,
	rememberId,
	rememberLastAccount,
	setDefaultAccount,
	startingAccount
} from './remember';

const store = new Map<string, string>();

beforeEach(() => {
	store.clear();
	vi.stubGlobal('localStorage', {
		getItem: (k: string) => store.get(k) ?? null,
		setItem: (k: string, v: string) => void store.set(k, v),
		removeItem: (k: string) => void store.delete(k)
	});
});

describe('remember', () => {
	it('round-trips an id', () => {
		rememberId('account', 7);
		expect(recallId('account')).toBe(7);
	});

	it('recalls nothing before anything is stored', () => {
		expect(recallId('account')).toBeNull();
	});

	it('uses the remembered id only while it is still an option', () => {
		rememberId('account', 3);
		expect(pickRemembered('account', [{ id: 1 }, { id: 3 }], 1)).toBe(3);
		expect(pickRemembered('account', [{ id: 1 }, { id: 2 }], 1)).toBe(1);
	});

	it('falls back when storage throws', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => {
				throw new Error('blocked');
			},
			setItem: () => {
				throw new Error('blocked');
			}
		});
		expect(() => rememberId('account', 3)).not.toThrow();
		expect(pickRemembered('account', [{ id: 3 }], 9)).toBe(9);
	});

	it('starts on the pinned account over the last one used', () => {
		const accounts = [{ id: 1 }, { id: 2 }, { id: 3 }];
		expect(startingAccount(accounts)).toBe(1);
		rememberLastAccount(2);
		expect(startingAccount(accounts)).toBe(2);
		setDefaultAccount(3);
		expect(defaultAccount()).toBe(3);
		expect(startingAccount(accounts)).toBe(3);
		// Logging elsewhere once doesn't move a pinned default.
		rememberLastAccount(1);
		expect(startingAccount(accounts)).toBe(3);
	});

	it('falls back to the last account when the pin is cleared or its account is gone', () => {
		rememberLastAccount(2);
		setDefaultAccount(3);
		expect(startingAccount([{ id: 1 }, { id: 2 }])).toBe(2);
		setDefaultAccount(null);
		expect(defaultAccount()).toBeNull();
		expect(startingAccount([{ id: 1 }, { id: 2 }, { id: 3 }])).toBe(2);
	});
});
