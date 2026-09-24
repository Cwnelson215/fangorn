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
