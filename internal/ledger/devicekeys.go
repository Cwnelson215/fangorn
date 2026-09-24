package ledger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/cwnelson/fangorn/internal/models"
)

// A device key is what an iPhone Shortcut sends in place of the login cookie.
// See migration 010 for why keys are per phone and deliberately narrow.

// DeviceKeyPrefix marks a string as a Fangorn key, so one pasted into the wrong
// place (a chat, a log) is recognisable for what it is.
const DeviceKeyPrefix = "fgn_"

func hashDeviceKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateDeviceKey makes a key for one phone and returns it with the plaintext
// token. The token is not stored and can't be shown again.
func (s *Service) CreateDeviceKey(ctx context.Context, householdID int, name string, accountID int) (models.DeviceKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.DeviceKey{}, "", invalid("give the phone a name, like “Carter's iPhone”")
	}
	if len(name) > 60 {
		return models.DeviceKey{}, "", invalid("keep the name under 60 characters")
	}
	if err := s.assertAccount(ctx, householdID, accountID); err != nil {
		return models.DeviceKey{}, "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return models.DeviceKey{}, "", fmt.Errorf("generating device key: %w", err)
	}
	token := DeviceKeyPrefix + base64.RawURLEncoding.EncodeToString(raw)

	var id int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO device_keys (household_id, name, account_id, token_hash)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		householdID, name, accountID, hashDeviceKey(token)).Scan(&id)
	if err != nil {
		return models.DeviceKey{}, "", fmt.Errorf("creating device key: %w", err)
	}
	key, err := s.getDeviceKey(ctx, householdID, id)
	return key, token, err
}

const deviceKeySelect = `
	SELECT k.id, k.household_id, k.name, k.account_id, a.name, k.created_at, k.last_used_at
	FROM device_keys k JOIN accounts a ON a.id = k.account_id`

func scanDeviceKey(row interface{ Scan(...any) error }) (models.DeviceKey, error) {
	var k models.DeviceKey
	var lastUsed sql.NullTime
	if err := row.Scan(&k.ID, &k.HouseholdID, &k.Name, &k.AccountID, &k.AccountName, &k.CreatedAt, &lastUsed); err != nil {
		return models.DeviceKey{}, err
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	return k, nil
}

func (s *Service) getDeviceKey(ctx context.Context, householdID, id int) (models.DeviceKey, error) {
	k, err := scanDeviceKey(s.db.QueryRowContext(ctx,
		deviceKeySelect+` WHERE k.id = $1 AND k.household_id = $2`, id, householdID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.DeviceKey{}, ErrNotFound
	}
	if err != nil {
		return models.DeviceKey{}, fmt.Errorf("loading device key: %w", err)
	}
	return k, nil
}

func (s *Service) ListDeviceKeys(ctx context.Context, householdID int) ([]models.DeviceKey, error) {
	rows, err := s.db.QueryContext(ctx,
		deviceKeySelect+` WHERE k.household_id = $1 ORDER BY k.created_at`, householdID)
	if err != nil {
		return nil, fmt.Errorf("listing device keys: %w", err)
	}
	defer rows.Close()
	out := []models.DeviceKey{}
	for rows.Next() {
		k, err := scanDeviceKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning device key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteDeviceKey revokes a key. The phone's Shortcut stops working at once.
func (s *Service) DeleteDeviceKey(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM device_keys WHERE id = $1 AND household_id = $2`, id, householdID)
	if err != nil {
		return fmt.Errorf("deleting device key: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeviceKeyByToken resolves a key from what a Shortcut sent and records that it
// was used. ErrNotFound covers a malformed, unknown and revoked key alike.
func (s *Service) DeviceKeyByToken(ctx context.Context, token string) (models.DeviceKey, error) {
	if !strings.HasPrefix(token, DeviceKeyPrefix) {
		return models.DeviceKey{}, ErrNotFound
	}
	var id, householdID int
	err := s.db.QueryRowContext(ctx,
		`UPDATE device_keys SET last_used_at = now() WHERE token_hash = $1 RETURNING id, household_id`,
		hashDeviceKey(token)).Scan(&id, &householdID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.DeviceKey{}, ErrNotFound
	}
	if err != nil {
		return models.DeviceKey{}, fmt.Errorf("checking device key: %w", err)
	}
	return s.getDeviceKey(ctx, householdID, id)
}

// ShortcutCategories lists the names a Shortcut offers to pick from, most used
// in the last 90 days first — the same order the app's quick-log screen uses.
func (s *Service) ShortcutCategories(ctx context.Context, householdID int, kind string) ([]string, error) {
	if kind != models.KindIncome && kind != models.KindExpense {
		return nil, invalid("kind must be %q or %q", models.KindExpense, models.KindIncome)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.name
		 FROM categories c
		 LEFT JOIN transactions t
		   ON t.category_id = c.id AND t.household_id = c.household_id
		  AND t.date >= CURRENT_DATE - 90
		 WHERE c.household_id = $1 AND c.kind = $2 AND c.archived_at IS NULL
		 GROUP BY c.id, c.name
		 ORDER BY count(t.id) DESC, c.name`, householdID, kind)
	if err != nil {
		return nil, fmt.Errorf("listing shortcut categories: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning category name: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ShortcutEntry is what a Shortcut sends. Categories come by name because a
// Shortcut picks from the list of names; the kind follows from the category
// unless it is a refund, which has to be asked for.
type ShortcutEntry struct {
	Amount   float64
	Category string
	Note     string
	Refund   bool
}

// LogFromShortcut records one entry on the key's account, dated today in the
// household's timezone.
func (s *Service) LogFromShortcut(ctx context.Context, key models.DeviceKey, in ShortcutEntry) (models.Transaction, error) {
	kind := models.KindExpense
	var categoryID *int
	description := strings.TrimSpace(in.Note)

	if name := strings.TrimSpace(in.Category); name != "" {
		var id int
		var catKind string
		err := s.db.QueryRowContext(ctx,
			`SELECT id, kind, name FROM categories
			 WHERE household_id = $1 AND archived_at IS NULL AND lower(name) = lower($2)`,
			key.HouseholdID, name).Scan(&id, &catKind, &name)
		if errors.Is(err, sql.ErrNoRows) {
			return models.Transaction{}, invalid("there's no category called “%s”", name)
		}
		if err != nil {
			return models.Transaction{}, fmt.Errorf("finding category: %w", err)
		}
		categoryID = &id
		kind = catKind
		if description == "" {
			description = name
		}
	}
	if in.Refund {
		if kind != models.KindExpense || categoryID == nil {
			return models.Transaction{}, invalid("a refund needs the spending category it came back from")
		}
		kind = models.KindRefund
	}
	if description == "" {
		description = "Expense"
	}

	h, err := s.household(ctx, key.HouseholdID)
	if err != nil {
		return models.Transaction{}, err
	}
	return s.CreateTransaction(ctx, key.HouseholdID, TransactionInput{
		AccountID:   key.AccountID,
		Date:        h.Today().Format(models.DateOnly),
		Amount:      in.Amount,
		Kind:        kind,
		Description: description,
		CategoryID:  categoryID,
	})
}
