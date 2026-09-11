package metadata

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrImportTooLarge = errors.New("reference import exceeds 100 MiB")
var ErrImportState = errors.New("operation not allowed in current reference import state")

const importBatchRecords = 256
const importBatchBytes = 1 << 20

// ReferenceImports owns complete uploads and their parsing checkpoints. The
// metadata worker consumes the stored references through its existing pacing,
// retry and successful-admission transaction.
type ReferenceImports struct {
	db      *sql.DB
	dir     string
	logger  *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	wake    chan struct{}
}

func NewReferenceImports(ctx context.Context, db *sql.DB, dir string, logger *slog.Logger) (*ReferenceImports, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create reference import directory: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &ReferenceImports{db: db, dir: dir, logger: logger, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	if err := s.recover(ctx); err != nil {
		cancel()
		return nil, err
	}
	s.workers.Go(s.run)
	return s, nil
}

func (s *ReferenceImports) Close() { s.cancel(); s.workers.Wait() }

func (s *ReferenceImports) path(id string) string { return filepath.Join(s.dir, id+".txt") }

func (s *ReferenceImports) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Accept publishes and syncs the complete file before committing ownership.
// A lost acknowledgement can leave an accepted import; every new upload is a
// separate submission. Partial transfers never become validation work.
func (s *ReferenceImports) Accept(ctx context.Context, filename string, body io.Reader) (collectorapi.ReferenceImport, error) {
	if err := s.ctx.Err(); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	file, err := os.CreateTemp(s.dir, "upload-*.part")
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	size, err := io.Copy(file, io.LimitReader(&importReader{ctx: ctx, reader: body}, collectorapi.MaxReferenceImportBytes+1))
	if err != nil {
		return collectorapi.ReferenceImport{}, fmt.Errorf("receive reference import: %w", err)
	}
	if size > collectorapi.MaxReferenceImportBytes {
		return collectorapi.ReferenceImport{}, ErrImportTooLarge
	}
	if err := file.Sync(); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if err := file.Close(); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	id := rand.Text()
	path := s.path(id)
	if err := os.Rename(file.Name(), path); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(path)
		}
	}()
	if err := syncImportDirectory(s.dir); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	at := time.Now().UTC()
	filename = filepath.Base(strings.ReplaceAll(filename, `\`, "/"))
	_, err = s.db.ExecContext(ctx, `INSERT INTO reference_imports
		(id, filename, status, created_at, size_bytes) VALUES (?, ?, 'processing', ?, ?)`, id, filename, at.UnixMilli(), size)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	committed = true
	s.signal()
	return collectorapi.ReferenceImport{ID: id, Filename: filename, Status: "processing", CreatedAt: at, SizeBytes: size}, nil
}

type importReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *importReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

const importColumns = `id, filename, status, created_at, completed_at, size_bytes,
	processed_bytes, reference_count, duplicates, invalid, known, imported, failed, pending, cancelled`

type importScanner interface{ Scan(...any) error }

func scanImport(row importScanner) (collectorapi.ReferenceImport, error) {
	var result collectorapi.ReferenceImport
	var created int64
	var completed sql.NullInt64
	err := row.Scan(&result.ID, &result.Filename, &result.Status, &created, &completed, &result.SizeBytes,
		&result.ProcessedBytes, &result.References, &result.Duplicates, &result.Invalid, &result.Known,
		&result.Imported, &result.Failed, &result.Pending, &result.Cancelled)
	result.CreatedAt = time.UnixMilli(created).UTC()
	result.CompletedAt = optionalTime(completed)
	return result, err
}

func (s *ReferenceImports) Get(ctx context.Context, id string) (collectorapi.ReferenceImport, error) {
	return scanImport(s.db.QueryRowContext(ctx, `SELECT `+importColumns+` FROM reference_imports
		WHERE id = ? AND (completed_at IS NULL OR completed_at > ?)`, id, time.Now().Add(-JobRetention).UnixMilli()))
}

func (s *ReferenceImports) List(ctx context.Context, limit, offset int64) ([]collectorapi.ReferenceImport, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+importColumns+` FROM reference_imports
		WHERE completed_at IS NULL OR completed_at > ? ORDER BY sequence DESC LIMIT ? OFFSET ?`,
		time.Now().Add(-JobRetention).UnixMilli(), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []collectorapi.ReferenceImport{}
	for rows.Next() {
		item, err := scanImport(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *ReferenceImports) Cancel(ctx context.Context, id string) (collectorapi.ReferenceImport, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	defer tx.Rollback()
	item, err := readImportTx(ctx, tx, id)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if item.Status == "cancelled" {
		return item, nil
	}
	if item.Status == "completed" {
		return collectorapi.ReferenceImport{}, ErrImportState
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_import_entries SET status = 'cancelled'
		WHERE import_id = ? AND status = 'pending'`, id); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_imports SET status = 'cancelled', completed_at = ? WHERE id = ?`, time.Now().UnixMilli(), id); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	item, err = readImportTx(ctx, tx, id)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if err := tx.Commit(); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	s.signal()
	return item, nil
}

// Retry changes only this import's failed outcomes. Matching inventory is
// reused, including previous collection failures; other imports retain their
// completed outcomes even when this attempt later confirms the same token.
func (s *ReferenceImports) Retry(ctx context.Context, id string) (collectorapi.ReferenceImport, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	defer tx.Rollback()
	item, err := readImportTx(ctx, tx, id)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if item.Status != "completed" || item.Failed == 0 {
		return collectorapi.ReferenceImport{}, ErrImportState
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_imports SET status = 'validating', completed_at = NULL WHERE id = ?`, id); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_import_entries SET status = CASE
		WHEN EXISTS (SELECT 1 FROM gallery_refs r WHERE r.gallery_id = reference_import_entries.gallery_id AND r.token = reference_import_entries.token) THEN 'known'
		WHEN EXISTS (SELECT 1 FROM gallery_refs r WHERE r.gallery_id = reference_import_entries.gallery_id) THEN 'failed'
		ELSE 'pending' END WHERE import_id = ? AND status = 'failed'`, id); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if err := completeReferenceImports(ctx, tx, time.Now()); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	item, err = readImportTx(ctx, tx, id)
	if err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	if err := tx.Commit(); err != nil {
		return collectorapi.ReferenceImport{}, err
	}
	s.signal()
	return item, nil
}

func readImportTx(ctx context.Context, tx *sql.Tx, id string) (collectorapi.ReferenceImport, error) {
	return scanImport(tx.QueryRowContext(ctx, `SELECT `+importColumns+` FROM reference_imports
		WHERE id = ? AND (completed_at IS NULL OR completed_at > ?)`, id, time.Now().Add(-JobRetention).UnixMilli()))
}

func syncImportDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Unowned publications and interrupted transfers were never acknowledged as
// accepted. Every processing row must still have its complete durable file.
func (s *ReferenceImports) recover(ctx context.Context) error {
	files, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		remove := strings.HasPrefix(name, "upload-") && strings.HasSuffix(name, ".part")
		if strings.HasSuffix(name, ".txt") {
			var owned int
			err := s.db.QueryRowContext(ctx, `SELECT 1 FROM reference_imports WHERE id = ?`, strings.TrimSuffix(name, ".txt")).Scan(&owned)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			remove = errors.Is(err, sql.ErrNoRows)
		}
		if remove {
			if err := os.Remove(filepath.Join(s.dir, name)); err != nil {
				return err
			}
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, size_bytes FROM reference_imports WHERE status = 'processing'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			return err
		}
		info, err := os.Stat(s.path(id))
		if err != nil {
			return fmt.Errorf("recover accepted reference import %s: %w", id, err)
		}
		if info.Size() != size {
			return fmt.Errorf("accepted reference import %s: file size changed", id)
		}
	}
	return rows.Err()
}

func (s *ReferenceImports) run() {
	for s.ctx.Err() == nil {
		worked, err := s.step(s.ctx)
		if err != nil && s.ctx.Err() == nil {
			s.logger.Error("reference_import_processing_failed", "error", err)
		}
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-s.ctx.Done():
		case <-s.wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

func (s *ReferenceImports) step(ctx context.Context) (bool, error) {
	// Deletion intent survives crashes between the final parsing checkpoint and
	// unlink. Cancellation uses the same cleanup path, after the parser closes.
	var cleanupID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM reference_imports WHERE has_file = 1 AND status != 'processing' LIMIT 1`).Scan(&cleanupID)
	if err == nil {
		if err := os.Remove(s.path(cleanupID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err := syncImportDirectory(s.dir); err != nil {
			return false, err
		}
		_, err := s.db.ExecContext(ctx, `UPDATE reference_imports SET has_file = 0 WHERE id = ?`, cleanupID)
		return true, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM reference_imports WHERE completed_at <= ? AND has_file = 0`, time.Now().Add(-JobRetention).UnixMilli()); err != nil {
		return false, err
	}
	var id string
	var offset int64
	err = s.db.QueryRowContext(ctx, `SELECT id, processed_bytes FROM reference_imports WHERE status = 'processing' ORDER BY sequence LIMIT 1`).Scan(&id, &offset)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file, err := os.Open(s.path(id))
	if err != nil {
		return false, err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return false, err
	}
	batch, err := readImportBatch(&importReader{ctx: ctx, reader: file})
	if err != nil {
		return false, err
	}
	return true, s.recordBatch(ctx, id, offset, batch)
}

type referenceImportBatch struct {
	refs    []panda.GalleryRef
	bytes   int64
	invalid int64
	done    bool
}

func readImportBatch(input io.Reader) (referenceImportBatch, error) {
	reader := bufio.NewReaderSize(input, 64<<10)
	var batch referenceImportBatch
	// Batch memory is bounded by both record count and bytes. A single record
	// may exceed the batch target, up to the accepted file's 100 MiB ceiling.
	for lines := 0; lines < importBatchRecords && batch.bytes < importBatchBytes; lines++ {
		line, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return referenceImportBatch{}, err
		}
		batch.bytes += int64(len(line))
		line = bytes.TrimSpace(line)
		if len(line) != 0 {
			idText, token, found := strings.Cut(string(line), ",")
			id, parseErr := strconv.ParseInt(idText, 10, 64)
			if !utf8.Valid(line) || !found || parseErr != nil || id <= 0 ||
				token == "" || strings.Contains(token, ",") || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
				batch.invalid++
			} else {
				batch.refs = append(batch.refs, panda.GalleryRef{ID: id, Token: token})
			}
		}
		if errors.Is(err, io.EOF) {
			batch.done = true
			break
		}
	}
	return batch, nil
}

func (s *ReferenceImports) recordBatch(ctx context.Context, id string, offset int64, batch referenceImportBatch) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int64
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status, processed_bytes FROM reference_imports WHERE id = ?`, id).Scan(&status, &current); err != nil {
		return err
	}
	if status != "processing" || current != offset {
		return nil
	}
	insert, err := tx.PrepareContext(ctx, `INSERT INTO reference_import_entries (import_id, gallery_id, token, status)
		VALUES (?, ?, ?, CASE
		WHEN EXISTS (SELECT 1 FROM gallery_refs WHERE gallery_id = ? AND token = ?) THEN 'known'
		WHEN EXISTS (SELECT 1 FROM gallery_refs WHERE gallery_id = ?) THEN 'failed'
		ELSE 'pending' END) ON CONFLICT (import_id, gallery_id, token) DO NOTHING`)
	if err != nil {
		return err
	}
	defer insert.Close()
	var duplicates int64
	for _, ref := range batch.refs {
		result, err := insert.ExecContext(ctx, id, ref.ID, ref.Token, ref.ID, ref.Token, ref.ID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		duplicates += 1 - count
	}
	if batch.done {
		status = "validating"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_imports SET processed_bytes = ?, status = ?,
		duplicates = duplicates + ?, invalid = invalid + ? WHERE id = ?`, offset+batch.bytes, status, duplicates, batch.invalid, id); err != nil {
		return err
	}
	if err := completeReferenceImports(ctx, tx, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func completeReferenceImports(ctx context.Context, tx *sql.Tx, at time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE reference_imports SET status = 'completed', completed_at = ?
		WHERE status = 'validating' AND pending = 0`, at.UnixMilli())
	return err
}
