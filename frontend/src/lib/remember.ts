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
