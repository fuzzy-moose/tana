package downloads

import (
	"context"
	"errors"
	"math"
	"os"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/downloads/dbgen"
)

var errStoragePaused = errors.New("download storage paused")

type StorageStatus struct {
	Paused          bool   `json:"paused"`
	Reason          string `json:"reason,omitempty"`
	AvailableBytes  *int64 `json:"available_bytes"`
	PauseBelowBytes int64  `json:"pause_below_bytes"`
	ResumeAtBytes   int64  `json:"resume_at_bytes"`
}

// Storage state and observations are protected by the service's state lock.
func (s *Service) storageStatus() StorageStatus {
	return StorageStatus{Paused: s.storage.Reason != "", Reason: s.storage.Reason,
		AvailableBytes: s.availableBytes, PauseBelowBytes: s.storageConfig.PauseBelowBytes,
		ResumeAtBytes: s.resumeAtBytes()}
}

func (s *Service) resumeAtBytes() int64 {
	return s.storageConfig.ResumeAtBytes + min(s.storage.ArchiveBytes, math.MaxInt64-s.storageConfig.ResumeAtBytes)
}

func (s *Service) saveStorage(ctx context.Context) error {
	if s.storage == s.savedStorage {
		return nil
	}
	if err := s.q.UpdateDownloadStorage(ctx, dbgen.UpdateDownloadStorageParams{
		Reason: s.storage.Reason, ArchiveBytes: s.storage.ArchiveBytes, GalleryID: s.storage.GalleryID,
	}); err != nil {
		return err
	}
	s.savedStorage = s.storage
	return nil
}

func (s *Service) checkStorage(ctx context.Context, allowResume bool) error {
	available, err := s.space(s.dir)
	s.availableBytes = nil
	if err != nil {
		s.storage.Reason = "space_check_failed"
	} else {
		s.availableBytes = &available
		switch {
		case available < s.storageConfig.PauseBelowBytes:
			s.storage.Reason = "low_space"
		case s.storage.Reason != "":
			if allowResume && available-s.storage.ArchiveBytes >= s.storageConfig.ResumeAtBytes {
				s.storage.Reason, s.storage.ArchiveBytes, s.storage.GalleryID = "", 0, 0
			} else {
				s.storage.Reason = "low_space"
			}
		}
	}
	if s.storage.Reason != "" && s.active != nil {
		s.storage.ArchiveBytes = max(s.storage.ArchiveBytes, s.active.expectedBytes)
		s.storage.GalleryID = s.active.id
		// Interrupt even if persisting the pause fails. Cleanup waits for persistence.
		s.active.cancel()
	}
	return s.saveStorage(ctx)
}

// Keep checking during archive preparation, transfer, and validation, including
// stalled network reads. The caller joins this monitor before publishing a file.
func (s *Service) monitorStorage(ctx context.Context, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			err := s.checkStorage(s.ctx, false)
			s.mu.Unlock()
			if err != nil && s.ctx.Err() == nil {
				s.logger.Error("download_storage_pause_failed", "error", err)
			}
		}
	}
}

// Admission reserves the full archive before any body bytes reach disk. The
// same size is retained for recovery because interrupted transfers restart.
func (s *Service) admitArchive(ctx context.Context, id, bytes int64) error {
	if err := s.checkStorage(ctx, false); err != nil {
		return err
	}
	if s.storage.Reason == "" && *s.availableBytes-bytes < s.storageConfig.PauseBelowBytes {
		s.storage.Reason, s.storage.ArchiveBytes, s.storage.GalleryID = "low_space", bytes, id
		if s.active != nil {
			s.active.cancel()
		}
	}
	if err := s.saveStorage(ctx); err != nil {
		return err
	}
	if s.storage.Reason != "" {
		return errStoragePaused
	}
	return nil
}

func (s *Service) removePartial(ctx context.Context, id, expectedBytes int64) error {
	path := s.path(id) + ".part"
	if s.storage.Reason != "" {
		info, err := os.Stat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && (s.storage.GalleryID == 0 || s.storage.GalleryID == id) {
			// Unknown-length responses still must not restart solely because their
			// discarded partial freed space. Known sizes reserve the whole archive.
			s.storage.ArchiveBytes = max(s.storage.ArchiveBytes, expectedBytes, info.Size())
			s.storage.GalleryID = id
		}
		if err := s.saveStorage(ctx); err != nil {
			return err
		}
	}
	return removeFile(path)
}

func (s *Service) releaseArchive(ctx context.Context, id int64) error {
	if s.storage.GalleryID == id {
		s.storage.GalleryID, s.storage.ArchiveBytes = 0, 0
		if err := s.saveStorage(ctx); err != nil {
			return err
		}
		s.signal()
	}
	return nil
}
