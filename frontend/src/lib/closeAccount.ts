import type { Account } from './types';

/**
 * What has to be typed to delete an account: its institution, or its name when
 * no institution was recorded.
 */
export function confirmationPhrase(account: Pick<Account, 'name' | 'institution_name'>): string {
	const institution = account.institution_name?.trim();
	return institution ? institution : account.name.trim();
}

/**
 * Whether what was typed matches, ignoring case and runs of whitespace — the
 * same way institutions are grouped, so "gesa  credit union" confirms "Gesa
 * Credit Union".
 */
export function matchesConfirmation(typed: string, phrase: string): boolean {
	const norm = (s: string) => s.trim().replace(/\s+/g, ' ').toLowerCase();
	return norm(phrase) !== '' && norm(typed) === norm(phrase);
}
