package metadata

import (
	"context"
	"sync"

	"github.com/fuzzy-moose/tana/internal/panda"
)

// Batch reserves gallery identities across the main fetcher and proxy channels.
// The caller must close it after fetching or abandoning it. Uncommitted work
// remains pending in storage, so reservations need not survive process exit.
type Batch struct {
	service *Service
	refs    []panda.GalleryRef
	closed  sync.Once
}

func (b *Batch) Size() int { return len(b.refs) }

// Fetch validates and commits a proxy batch without changing main retry state.
// Request-level errors leave the galleries pending, including proxy rejections.
func (b *Batch) Fetch(ctx context.Context, client *panda.Client) error {
	return b.fetch(ctx, client, false)
}

func (b *Batch) Close() {
	b.closed.Do(func() {
		b.service.dispatch.Lock()
		defer b.service.dispatch.Unlock()
		for _, ref := range b.refs {
			delete(b.service.reserved, ref.ID)
		}
	})
}

// ClaimBackground selects inventory first, then imports, independently of
// explicit jobs and the main fetcher's cooldown. Nil means no eligible work.
func (s *Service) ClaimBackground(ctx context.Context) (*Batch, error) {
	return s.claim(ctx, true)
}

func (s *Service) claim(ctx context.Context, background bool) (*Batch, error) {
	s.dispatch.Lock()
	defer s.dispatch.Unlock()
	refs, err := s.store.pendingRefsFor(ctx, background, s.reserved)
	if err != nil || len(refs) == 0 {
		return nil, err
	}
	if s.reserved == nil {
		s.reserved = make(map[int64]bool)
	}
	for _, ref := range refs {
		s.reserved[ref.ID] = true
	}
	return &Batch{service: s, refs: refs}, nil
}
