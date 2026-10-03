// Per-device form defaults. Logging on a phone is mostly the same card over
// and over, so the entry forms start from whatever account was used last.
//
// This is a convenience, not data: storage can be missing or throw (private
// browsing, blocked site data), and every caller falls back to a default.

const PREFIX = 'fangorn.';

export function recallId(key: string): number | null {
	try {
		const n = Number(localStorage.getItem(PREFIX + key));
		return Number.isInteger(n) && n > 0 ? n : null;
	} catch {
		return null;
	}
}

export function rememberId(key: string, id: number): void {
	try {
		localStorage.setItem(PREFIX + key, String(id));
	} catch {
		// Not remembering is fine.
	}
}

/**
 * The remembered id if it is still one of the options (an account may have
 * been archived or deleted since), otherwise the fallback.
 */
export function pickRemembered(key: string, options: { id: number }[], fallback: number): number {
	const id = recallId(key);
	return id !== null && options.some((o) => o.id === id) ? id : fallback;
}

// The account a hand-logged entry starts on. Settings can pin one for this
// device; without a pin (or once the pinned account is gone) it is whichever
// account was logged to last.
const DEFAULT_ACCOUNT = 'transaction.default';
const LAST_ACCOUNT = 'transaction.account';

export function startingAccount(accounts: { id: number }[]): number {
	const last = pickRemembered(LAST_ACCOUNT, accounts, accounts[0]?.id ?? 0);
	return pickRemembered(DEFAULT_ACCOUNT, accounts, last);
}

export function rememberLastAccount(id: number): void {
	rememberId(LAST_ACCOUNT, id);
}

/** This device's pinned account, or null when it follows the last one used. */
export function defaultAccount(): number | null {
	return recallId(DEFAULT_ACCOUNT);
}

export function setDefaultAccount(id: number | null): void {
	if (id === null) forget(DEFAULT_ACCOUNT);
	else rememberId(DEFAULT_ACCOUNT, id);
}

/**
 * A remembered set of values — a projection's assumptions — or null. Only
 * fields whose type matches the fallback are taken, so a stale shape from an
 * older version can't leak a string into a slider.
 */
export function recallValues<T extends Record<string, number | boolean>>(key: string, fallback: T): T {
	try {
		const raw = JSON.parse(localStorage.getItem(PREFIX + key) ?? 'null');
		if (!raw || typeof raw !== 'object') return fallback;
		const out = { ...fallback };
		for (const k of Object.keys(fallback) as (keyof T)[]) {
			const v = raw[k];
			if (typeof v === typeof fallback[k] && (typeof v !== 'number' || Number.isFinite(v))) out[k] = v;
		}
		return out;
	} catch {
		return fallback;
	}
}

export function rememberValues(key: string, values: Record<string, number | boolean>): void {
	try {
		localStorage.setItem(PREFIX + key, JSON.stringify(values));
	} catch {
		// Not remembering is fine.
	}
}

export function forget(key: string): void {
	try {
		localStorage.removeItem(PREFIX + key);
	} catch {
		// Nothing to forget.
	}
}
