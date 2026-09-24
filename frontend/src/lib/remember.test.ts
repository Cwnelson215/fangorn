import { beforeEach, describe, expect, it, vi } from 'vitest';
import { pickRemembered, recallId, rememberId } from './remember';

const store = new Map<string, string>();

beforeEach(() => {
	store.clear();
	vi.stubGlobal('localStorage', {
		getItem: (k: string) => store.get(k) ?? null,
		setItem: (k: string, v: string) => void store.set(k, v)
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
});
