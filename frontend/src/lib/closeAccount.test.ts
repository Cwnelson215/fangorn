import { describe, expect, it } from 'vitest';
import { confirmationPhrase, matchesConfirmation } from './closeAccount';

describe('confirmationPhrase', () => {
	it('asks for the institution', () => {
		expect(confirmationPhrase({ name: 'Checking', institution_name: ' Gesa Credit Union ' })).toBe(
			'Gesa Credit Union'
		);
	});

	it('falls back to the account name', () => {
		expect(confirmationPhrase({ name: 'Cash jar', institution_name: null })).toBe('Cash jar');
		expect(confirmationPhrase({ name: 'Cash jar', institution_name: '  ' })).toBe('Cash jar');
	});
});

describe('matchesConfirmation', () => {
	it('ignores case and extra spaces', () => {
		expect(matchesConfirmation('  gesa   credit union', 'Gesa Credit Union')).toBe(true);
	});

	it('rejects anything else', () => {
		expect(matchesConfirmation('Gesa', 'Gesa Credit Union')).toBe(false);
		expect(matchesConfirmation('', '')).toBe(false);
	});
});
