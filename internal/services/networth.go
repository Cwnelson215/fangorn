package services

import (
	"context"
	"database/sql"
	"time"
)

// SnapshotNetWorth calculates current net worth from account balances and upserts a daily snapshot.
func SnapshotNetWorth(ctx context.Context, db *sql.DB) error {
	var assets, liabilities float64

	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE WHEN type IN ('depository', 'investment') THEN COALESCE(current_balance, 0) ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN type IN ('credit', 'loan') THEN COALESCE(current_balance, 0) ELSE 0 END), 0)
		 FROM accounts`,
	).Scan(&assets, &liabilities)
	if err != nil {
		return err
	}

	today := time.Now().Format("2006-01-02")
	_, err = db.ExecContext(ctx,
		`INSERT INTO net_worth_snapshots (total_assets, total_liabilities, net_worth, snapshot_date)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (snapshot_date) DO UPDATE SET
		   total_assets = EXCLUDED.total_assets, total_liabilities = EXCLUDED.total_liabilities,
		   net_worth = EXCLUDED.net_worth`,
		assets, liabilities, assets-liabilities, today,
	)
	return err
}
