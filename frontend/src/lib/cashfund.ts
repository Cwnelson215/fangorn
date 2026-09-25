// Where a brokerage keeps an account's uninvested cash by default. The account
// form fills this in from the institution so a new Fidelity account starts
// with SPAXX without anyone having to know that; it can always be changed.
// Only brokerages whose default core position is a money market fund belong
// here — a bank sweep earns no fund yield to look up.

const DEFAULT_CASH_FUNDS: [RegExp, string][] = [
	[/fidelity/i, 'SPAXX'],
	[/vanguard/i, 'VMFXX']
];

export function defaultCashFund(institution: string | null | undefined): string | null {
	if (!institution) return null;
	for (const [pattern, fund] of DEFAULT_CASH_FUNDS) {
		if (pattern.test(institution)) return fund;
	}
	return null;
}
