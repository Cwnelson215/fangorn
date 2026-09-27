package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cwnelson/fangorn/internal/models"
)

// ErrChatChanged means a chat gained messages between being read and being
// appended to — the same conversation answered twice at once, from two tabs or
// two phones. The later answer is dropped rather than interleaved, because a
// transcript with two replies to one question is not one the API will accept.
var ErrChatChanged = errors.New("chat changed while answering")

const chatSelect = `SELECT id, title, created_at, updated_at FROM chats`

func scanChat(row interface{ Scan(...any) error }) (models.Chat, error) {
	var c models.Chat
	var created, updated time.Time
	if err := row.Scan(&c.ID, &c.Title, &created, &updated); err != nil {
		return c, err
	}
	c.CreatedAt = created.Format(time.RFC3339)
	c.UpdatedAt = updated.Format(time.RFC3339)
	return c, nil
}

// ListChats returns the household's chats, most recently used first. A chat
// whose first question failed has no messages and isn't listed.
func (s *Service) ListChats(ctx context.Context, householdID int) ([]models.Chat, error) {
	rows, err := s.db.QueryContext(ctx,
		chatSelect+` WHERE household_id = $1 AND jsonb_array_length(messages) > 0
		 ORDER BY updated_at DESC, id DESC LIMIT 200`, householdID)
	if err != nil {
		return nil, fmt.Errorf("listing chats: %w", err)
	}
	defer rows.Close()

	out := []models.Chat{}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning chat: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateChat starts an empty conversation.
func (s *Service) CreateChat(ctx context.Context, householdID int) (models.Chat, error) {
	c, err := scanChat(s.db.QueryRowContext(ctx,
		`INSERT INTO chats (household_id) VALUES ($1) RETURNING id, title, created_at, updated_at`, householdID))
	if err != nil {
		return c, fmt.Errorf("creating chat: %w", err)
	}
	return c, nil
}

// ChatTranscript returns a chat and its Messages API history, one raw message
// per element, exactly as stored.
func (s *Service) ChatTranscript(ctx context.Context, householdID, id int) (models.Chat, []json.RawMessage, error) {
	var raw []byte
	var c models.Chat
	var created, updated time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, created_at, updated_at, messages FROM chats WHERE id = $1 AND household_id = $2`,
		id, householdID,
	).Scan(&c.ID, &c.Title, &created, &updated, &raw)
	if err == sql.ErrNoRows {
		return c, nil, ErrNotFound
	}
	if err != nil {
		return c, nil, fmt.Errorf("loading chat: %w", err)
	}
	c.CreatedAt = created.Format(time.RFC3339)
	c.UpdatedAt = updated.Format(time.RFC3339)

	var msgs []json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return c, nil, fmt.Errorf("decoding chat %d transcript: %w", id, err)
	}
	return c, msgs, nil
}

// AppendChat adds one finished turn to a chat's transcript. expectedLen is the
// transcript length the turn was built on; if the chat has moved on since, the
// append is refused with ErrChatChanged. A chat still without a title takes
// title.
func (s *Service) AppendChat(ctx context.Context, householdID, id, expectedLen int, msgs []json.RawMessage, title string) error {
	if len(msgs) == 0 {
		return nil
	}
	add, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("encoding chat messages: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE chats
		    SET messages = messages || $3::jsonb,
		        title = CASE WHEN title = '' THEN $4 ELSE title END,
		        updated_at = now()
		  WHERE id = $1 AND household_id = $2 AND jsonb_array_length(messages) = $5`,
		id, householdID, string(add), title, expectedLen)
	if err != nil {
		return fmt.Errorf("appending to chat: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM chats WHERE id = $1 AND household_id = $2)`, id, householdID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("checking chat: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return ErrChatChanged
}

func (s *Service) DeleteChat(ctx context.Context, householdID, id int) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM chats WHERE id = $1 AND household_id = $2`, id, householdID)
	if err != nil {
		return fmt.Errorf("deleting chat: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
