package delivery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/source"
)

const transferInactivityTimeout = 30 * time.Second

func newToken() string { return rand.Text() }

func (s *Service) deliver(b Batch, index int) error {
	item := b.Items[index]
	if item.State == "completed" || item.State == "skipped" || item.State == "failed" {
		return nil
	}
	if err := s.available(s.ctx, b); err != nil {
		return s.pause(b, err)
	}
	if item.State == "cleanup_pending" {
		return s.cleanup(b, index)
	}
	err := s.transferAndImport(b, &item)
	if err != nil {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		if unavailable := s.available(s.ctx, b); unavailable != nil {
			return s.pause(b, unavailable)
		}
		item.State, item.Error = "failed", err.Error()
		return s.updateItem(b, item)
	}
	if item.State == "skipped" {
		return nil
	}
	b.Items[index] = item
	return s.cleanup(b, index)
}

func (s *Service) transferAndImport(b Batch, item *Item) error {
	if item.checkpoint.Stage == "queued" {
		present, err := s.present(s.ctx, item.GalleryID)
		if err != nil {
			return err
		}
		if present {
			item.State, item.Error = "skipped", "Already in library"
			return s.updateItem(b, *item)
		}
		if _, err := os.Lstat(filepath.Join(b.root, archiveName(item.GalleryID))); err == nil {
			return errors.New("destination file already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		item.State = "transferring"
		if err := s.updateItem(b, *item); err != nil {
			return err
		}
		if err := s.download(b, item); err != nil {
			return err
		}
		item.State, item.checkpoint.Stage = "transferred", "transferred"
		if err := s.updateItem(b, *item); err != nil {
			return err
		}
	}
	if item.checkpoint.Stage == "transferred" {
		if err := finalize(b, *item); err != nil {
			return err
		}
		item.State, item.checkpoint.Stage = "saved", "saved"
		if err := s.updateItem(b, *item); err != nil {
			return err
		}
	}
	if item.checkpoint.Stage == "saved" {
		if err := verifyArchive(s.ctx, b, *item); err != nil {
			return err
		}
		// Once saved is durable, the unique staging link is no longer needed to
		// recognize a final file left by an interrupted atomic link operation.
		if err := os.Remove(stagingPath(b, *item)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove delivery staging file: %w", err)
		}
		item.State = "importing"
		if err := s.updateItem(b, *item); err != nil {
			return err
		}
		if err := s.importer.ImportArchive(s.ctx, b.LibraryID, archiveName(item.GalleryID)); err != nil {
			return fmt.Errorf("import archive: %w", err)
		}
		item.State, item.Error, item.checkpoint.Stage = "cleanup_pending", "", "cleanup_pending"
		if err := s.updateItem(b, *item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) download(b Batch, item *Item) error {
	stage := stagingPath(b, *item)
	dir := filepath.Dir(stage)
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create delivery staging directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("delivery staging path is not a directory")
	}
	// Persist the staging directory's entry before a transferred checkpoint may
	// rely on it surviving a crash, including on the library's first delivery.
	if err := syncDirectory(b.root); err != nil {
		return err
	}
	// A queued checkpoint owns only this random staging name; interrupted copies
	// can be discarded without touching a final archive or another delivery.
	if err := os.Remove(stage); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(stage)
		}
	}()
	ctx, cancel := context.WithCancelCause(s.ctx)
	defer cancel(nil)
	response, err := s.collector.OpenDownload(ctx, item.GalleryID, http.MethodGet, nil)
	if err != nil {
		return fmt.Errorf("retrieve collector archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("collector archive returned HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	// Cancel the request to interrupt a blocked body read; a context check before
	// Read alone cannot release it. Receiving bytes extends the deadline so large,
	// active archives have no total transfer limit.
	idle := time.AfterFunc(transferInactivityTimeout, func() {
		cancel(fmt.Errorf("collector archive transfer inactive for %s", transferInactivityTimeout))
	})
	size, err := io.Copy(io.MultiWriter(file, hash), transferReader{contextReader{ctx: ctx, reader: response.Body}, idle})
	idle.Stop()
	if cause := context.Cause(ctx); cause != nil {
		err = cause
	}
	if err != nil {
		return fmt.Errorf("save archive: %w", err)
	}
	if response.ContentLength >= 0 && size != response.ContentLength {
		return errors.New("collector archive length does not match response")
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync archive: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	item.checkpoint.Size, item.checkpoint.SHA256 = size, fmt.Sprintf("%x", hash.Sum(nil))
	complete = true
	return nil
}

// A hard link publishes a complete archive without overwriting a preexisting
// path. The staging link survives until the saved checkpoint is committed.
func finalize(b Batch, item Item) error {
	stage, destination := stagingPath(b, item), filepath.Join(b.root, archiveName(item.GalleryID))
	info, err := os.Lstat(stage)
	if err != nil {
		return fmt.Errorf("read saved staging archive: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("delivery staging archive is not a regular file")
	}
	if err := os.Link(stage, destination); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("publish archive: %w", err)
		}
		final, statErr := os.Lstat(destination)
		if statErr != nil || !final.Mode().IsRegular() || !os.SameFile(info, final) {
			return errors.New("destination file already exists")
		}
	}
	return syncDirectory(b.root)
}

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func verifyArchive(ctx context.Context, b Batch, item Item) error {
	path := filepath.Join(b.root, archiveName(item.GalleryID))
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify saved archive: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != item.checkpoint.Size || item.checkpoint.SHA256 == "" {
		return errors.New("saved archive changed; collector copy retained")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errors.New("saved archive changed; collector copy retained")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, reader: file}); err != nil {
		return err
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != item.checkpoint.SHA256 {
		return errors.New("saved archive changed; collector copy retained")
	}
	return nil
}

func (s *Service) cleanup(b Batch, index int) error {
	item := b.Items[index]
	if item.CleanupAttempts >= maxCleanupAttempts {
		return nil
	}
	err := s.available(s.ctx, b)
	if err != nil && b.State == "running" {
		return s.pause(b, err)
	}
	if err == nil {
		err = verifyArchive(s.ctx, b, item)
	}
	if err == nil {
		var imported bool
		err = s.db.QueryRowContext(s.ctx, "SELECT EXISTS(SELECT 1 FROM sources WHERE library_id = ? AND path = ? AND kind = 'archive')", b.LibraryID, archiveName(item.GalleryID)).Scan(&imported)
		if err == nil && !imported {
			err = errors.New("imported source is missing; collector copy retained")
		}
	}
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	// Persist the attempt before the remote call so failures remain bounded over
	// restarts. A lost successful response is recovered by accepting HTTP 404.
	item.CleanupAttempts++
	item.checkpoint.NextCleanup = time.Now().Add(time.Duration(item.CleanupAttempts*item.CleanupAttempts) * 5 * time.Second)
	if updateErr := s.updateItem(b, item); updateErr != nil {
		return updateErr
	}
	if err == nil {
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		err = s.collector.DeleteDownload(ctx, item.GalleryID)
		cancel()
		var remote *collectorapi.HTTPError
		if errors.As(err, &remote) && remote.StatusCode == http.StatusNotFound {
			err = nil
		}
	}
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if err == nil {
		item.State, item.Error, item.checkpoint.Stage = "completed", "", "completed"
	} else {
		item.State, item.Error = "cleanup_pending", err.Error()
	}
	return s.updateItem(b, item)
}

func (s *Service) present(ctx context.Context, id int64) (bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT s.path, s.kind, l.path FROM sources s JOIN libraries l ON l.id = s.library_id")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, root string
		var kind source.Kind
		if err := rows.Scan(&name, &kind, &root); err != nil {
			return false, err
		}
		if kind == source.Directory && name == "." {
			name = filepath.Base(root)
		}
		if source.PandaCandidateID(name, kind) == id {
			return true, nil
		}
	}
	return false, rows.Err()
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

type transferReader struct {
	contextReader
	idle *time.Timer
}

func (r transferReader) Read(p []byte) (int, error) {
	n, err := r.contextReader.Read(p)
	if n > 0 {
		r.idle.Reset(transferInactivityTimeout)
	}
	return n, err
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
