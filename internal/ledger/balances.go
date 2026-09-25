package ledger

// accountBalances is THE definition of what an account is worth. accountSelect,
// SnapshotNetWorth and goalSelect all join it, and nothing else may recompute a
// balance — when these lived in three places, adding holdings would have meant
// remembering all three, and the one that got missed would be silently wrong.
//
//	cash_balance    starting_balance + SUM(transactions.amount)
//	holdings_value  SUM over symbols of (net shares × latest price), per account
//
// Net shares are summed straight from the trade log: sells subtract, every other
// side adds. Each position is truncated to the cent before summing — how the
// brokerage values a position — which is the same cut portfolio.MarketValue
// applies in Go, so the two figures agree exactly.
//
// Callers select from it as a subquery aliased b, joined on b.account_id.
const accountBalances = `
	SELECT a.id AS account_id,
	       a.starting_balance + COALESCE(t.total, 0) AS cash_balance,
	       COALESCE(h.value, 0) AS holdings_value
	FROM accounts a
	LEFT JOIN (
		SELECT account_id, SUM(amount) AS total FROM transactions GROUP BY account_id
	) t ON t.account_id = a.id
	LEFT JOIN (
		SELECT p.account_id, SUM(TRUNC(p.shares * s.last_price, 2)) AS value
		FROM (
			SELECT account_id, symbol,
			       SUM(CASE WHEN side = 'sell' THEN -shares ELSE shares END) AS shares
			FROM trades
			GROUP BY account_id, symbol
		) p
		JOIN securities s ON s.symbol = p.symbol
		GROUP BY p.account_id
	) h ON h.account_id = a.id`
