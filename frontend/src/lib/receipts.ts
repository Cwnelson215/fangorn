import type { Receipt } from './types';

/**
 * Explains, in words, why a receipt is waiting for a person. The server sends
 * reason codes; the wording lives here because it needs the receipt's own
 * details to be useful ("no account ends in 4821", not "account unresolved").
 */
export function reasonText(reason: string, r: Receipt): string {
	switch (reason) {
		case 'missing_total':
			return "Couldn't read the total.";
		case 'missing_date':
			return "Couldn't read the date.";
		case 'no_category_match':
			return r.category_suggested
				? `Looks like “${r.category_suggested}”, which isn't one of your categories.`
				: "Couldn't tell which category this belongs in.";
		case 'account_unresolved':
			if (r.tender === 'card' && r.card_last4)
				return `Paid with a card ending ${r.card_last4}, and no account has those last 4 digits. Add them to the right account and future receipts will match on their own.`;
			if (r.tender === 'cash') return "Paid in cash, but it isn't clear which cash account.";
			return "Couldn't tell which account paid.";
		case 'not_a_receipt':
			return "This doesn't look like a receipt.";
		case 'looks_like_return':
			return 'This looks like a return, so it goes in as a refund.';
		case 'non_usd':
			return `Priced in ${r.currency ?? 'another currency'}. Enter what it cost in dollars.`;
		case 'possible_duplicate':
			return 'An expense for the same amount is already logged on this account within a day. It may already be in.';
		case 'date_out_of_range':
			return 'The date is in the future or more than 60 days ago. Check the year.';
		case 'totals_disagree':
			return "The subtotal, tax and tip don't add up to the total.";
		case 'unreadable':
			return "The photo couldn't be read.";
		case 'extraction_disabled':
			return 'Automatic reading is switched off, so this one is up to you.';
		case 'extraction_failed':
			return 'Reading it kept failing. Try again later, or enter it here.';
	}
	return reason;
}

export function isWorking(r: Receipt): boolean {
	return r.status === 'pending' || r.status === 'processing';
}
