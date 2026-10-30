package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cwnelson/fangorn/internal/models"
	"github.com/lib/pq"
)

// What the assistant knows beyond the ledger: the family's own instructions and
// the files they've attached, like a claude.ai Project's instructions and
// knowledge. Every new chat starts from a snapshot of them (SnapshotChatContext)
// and keeps it, so editing them changes chats started afterwards, not the ones
// already going.

const (
	// MaxInstructionRunes bounds the instructions. They go with every
	// question, so this is about cost as much as anything.
	MaxInstructionRunes = 50000

	// MaxAssistantFileBytes caps one file, and MaxAssistantFilesBytes all of
	// them together: every file is sent with every question, base64 for a PDF,
	// and one request can't exceed 32 MB.
	MaxAssistantFileBytes  = 10 << 20
	MaxAssistantFilesBytes = 20 << 20
	maxAssistantFiles      = 20
)

// AssistantContext is what the Ask page's settings show.
type AssistantContext struct {
	Instructions string                 `json:"instructions"`
	Files        []models.AssistantFile `json:"files"`
}

// ContextFile is an attached file with its bytes, to send to the model.
type ContextFile struct {
	ID        int
	Name      string
	MediaType string
	Data      []byte
}

func (s *Service) GetAssistantContext(ctx context.Context, householdID int) (AssistantContext, error) {
	var out AssistantContext
	err := s.db.QueryRowContext(ctx,
		`SELECT assistant_instructions FROM households WHERE id = $1`, householdID,
	).Scan(&out.Instructions)
	if err == sql.ErrNoRows {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("loading assistant instructions: %w", err)
	}
	if out.Files, err = s.ListAssistantFiles(ctx, householdID); err != nil {
		return out, err
	}
	return out, nil
}

// SetAssistantInstructions replaces the family's instructions. Blank clears them.
func (s *Service) SetAssistantInstructions(ctx context.Context, householdID int, text string) (string, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > MaxInstructionRunes {
		return "", invalid("keep the instructions under %d characters", MaxInstructionRunes)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE households SET assistant_instructions = $2 WHERE id = $1`, householdID, text)
	if err != nil {
		return "", fmt.Errorf("saving assistant instructions: %w", err)
	}
	return text, nil
}

func (s *Service) ListAssistantFiles(ctx context.Context, householdID int) ([]models.AssistantFile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, media_type, size, created_at FROM assistant_files
		 WHERE household_id = $1 AND removed_at IS NULL ORDER BY id`, householdID)
	if err != nil {
		return nil, fmt.Errorf("listing assistant files: %w", err)
	}
	defer rows.Close()
	out := []models.AssistantFile{}
	for rows.Next() {
		var f models.AssistantFile
		var created time.Time
		if err := rows.Scan(&f.ID, &f.Name, &f.MediaType, &f.Size, &created); err != nil {
			return nil, fmt.Errorf("scanning assistant file: %w", err)
		}
		f.CreatedAt = created.Format(time.RFC3339)
		out = append(out, f)
	}
	return out, rows.Err()
}

// AddAssistantFile attaches a file. mediaType is application/pdf or text/plain;
// the handler decides which from the bytes.
func (s *Service) AddAssistantFile(ctx context.Context, householdID int, name, mediaType string, data []byte) (models.AssistantFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled"
	}
	if utf8.RuneCountInString(name) > 200 {
		name = string([]rune(name)[:200])
	}
	if mediaType != "application/pdf" && mediaType != "text/plain" {
		return models.AssistantFile{}, invalid("only PDFs and plain text files can be attached")
	}
	if len(data) == 0 {
		return models.AssistantFile{}, invalid("that file is empty")
	}
	if len(data) > MaxAssistantFileBytes {
		return models.AssistantFile{}, invalid("keep each file under %d MB", MaxAssistantFileBytes>>20)
	}

	var f models.AssistantFile
	err := s.inTx(func(tx *sql.Tx) error {
		// Serialize uploads so two at once can't both fit under the total.
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM households WHERE id = $1 FOR UPDATE`, householdID); err != nil {
			return fmt.Errorf("locking household: %w", err)
		}
		var count, total int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM assistant_files
			 WHERE household_id = $1 AND removed_at IS NULL`, householdID,
		).Scan(&count, &total); err != nil {
			return fmt.Errorf("totalling assistant files: %w", err)
		}
		if count >= maxAssistantFiles {
			return invalid("there's a limit of %d files; remove one first", maxAssistantFiles)
		}
		if total+len(data) > MaxAssistantFilesBytes {
			return invalid("all the files together have to stay under %d MB; remove one first",
				MaxAssistantFilesBytes>>20)
		}
		var created time.Time
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO assistant_files (household_id, name, media_type, data, size)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
			householdID, name, mediaType, data, len(data),
		).Scan(&f.ID, &created); err != nil {
			return fmt.Errorf("saving assistant file: %w", err)
		}
		f.Name, f.MediaType, f.Size = name, mediaType, len(data)
		f.CreatedAt = created.Format(time.RFC3339)
		return nil
	})
	return f, err
}

// RemoveAssistantFile takes a file out of what new chats start with. A chat
// that already started with it keeps it, so its bytes stay until no chat
// uses it.
func (s *Service) RemoveAssistantFile(ctx context.Context, householdID, id int) error {
	return s.inTx(func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE assistant_files SET removed_at = now()
			 WHERE id = $1 AND household_id = $2 AND removed_at IS NULL`, id, householdID)
		if err != nil {
			return fmt.Errorf("removing assistant file: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return purgeRemovedFiles(ctx, tx, householdID)
	})
}

// purgeRemovedFiles deletes removed files that no chat uses any more.
func purgeRemovedFiles(ctx context.Context, tx *sql.Tx, householdID int) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM assistant_files f
		 WHERE f.household_id = $1 AND f.removed_at IS NOT NULL
		   AND NOT EXISTS (
		     SELECT 1 FROM chats c
		     WHERE c.household_id = f.household_id
		       AND c.context->'file_ids' @> jsonb_build_array(f.id))`, householdID)
	if err != nil {
		return fmt.Errorf("purging removed assistant files: %w", err)
	}
	return nil
}

// SnapshotChatContext is what a chat starting now begins with.
func (s *Service) SnapshotChatContext(ctx context.Context, householdID int) (models.ChatContext, error) {
	cur, err := s.GetAssistantContext(ctx, householdID)
	if err != nil {
		return models.ChatContext{}, err
	}
	cc := models.ChatContext{Instructions: cur.Instructions, FileIDs: []int{}}
	for _, f := range cur.Files {
		cc.FileIDs = append(cc.FileIDs, f.ID)
	}
	return cc, nil
}

// ContextFiles loads the files a chat's context names, removed or not, in id
// order — the order they were attached, and the same every turn.
func (s *Service) ContextFiles(ctx context.Context, householdID int, ids []int) ([]ContextFile, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ids64 := make([]int64, len(ids))
	for i, id := range ids {
		ids64[i] = int64(id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, media_type, data FROM assistant_files
		 WHERE household_id = $1 AND id = ANY($2) ORDER BY id`, householdID, pq.Array(ids64))
	if err != nil {
		return nil, fmt.Errorf("loading assistant files: %w", err)
	}
	defer rows.Close()
	var out []ContextFile
	for rows.Next() {
		var f ContextFile
		if err := rows.Scan(&f.ID, &f.Name, &f.MediaType, &f.Data); err != nil {
			return nil, fmt.Errorf("scanning assistant file: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
