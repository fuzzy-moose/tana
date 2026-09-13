package delivery

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/library"
)

const destinationProbeTimeout = 5 * time.Second

func (s *Service) Resume(ctx context.Context, id int64) (Batch, error) {
	b, err := s.Get(ctx, id)
	if err != nil {
		return Batch{}, err
	}
	if b.State != "paused" {
		return Batch{}, ErrInvalid
	}
	if err := s.available(ctx, b); err != nil {
		return Batch{}, err
	}
	return s.change(ctx, id, func(b *Batch) error {
		// The probe releases the mutex; revalidate in case another action won.
		if b.State != "paused" {
			return ErrInvalid
		}
		if err := s.noActive(ctx, id); err != nil {
			return err
		}
		b.State, b.Error, b.StopRequested = "running", "", false
		return nil
	})
}

func (s *Service) Retry(ctx context.Context, id int64) (Batch, error) {
	b, err := s.Get(ctx, id)
	if err != nil {
		return Batch{}, err
	}
	if b.State != "stopped" && b.State != "completed_with_errors" {
		return Batch{}, ErrInvalid
	}
	unavailable := s.available(ctx, b)
	return s.change(ctx, id, func(current *Batch) error {
		if current.State != b.State {
			return ErrInvalid
		}
		if err := s.noActive(ctx, id); err != nil {
			return err
		}
		for i := range current.Items {
			if current.Items[i].State == "failed" {
				current.Items[i].State, current.Items[i].Error = current.Items[i].checkpoint.Stage, ""
			}
		}
		current.State, current.Error, current.StopRequested = "running", "", false
		if unavailable != nil {
			current.State, current.Error = "paused", unavailable.Error()
		}
		return nil
	})
}

func (s *Service) available(ctx context.Context, b Batch) error {
	ctx, cancel := context.WithTimeout(ctx, destinationProbeTimeout)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	lib, err := s.libraries.Get(ctx, b.LibraryID)
	if err == nil && lib.Path != b.root {
		err = fmt.Errorf("library root changed")
	}
	if err == nil {
		s.probeOnce.Do(func() { s.probe = library.NewDirectoryProbe(os.DirFS) })
		err = s.probe.Check(ctx, b.root)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}
