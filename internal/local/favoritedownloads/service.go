// Package favoritedownloads reviews missing favorites against the local catalog
// before submitting their archives to the collector.
package favoritedownloads

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var (
	ErrInvalidCategory = errors.New("invalid favorite category")
	ErrInvalidPreview  = errors.New("missing favorite download preview expired or does not match the category")
)

const previewLifetime = 30 * time.Minute

type collectorClient interface {
	FavoriteDownloadCandidates(context.Context, int) ([]collectorapi.FavoriteDownloadCandidate, error)
	SubmitDownloads(context.Context, []panda.GalleryRef) (collectorapi.DownloadBatchCounts, error)
}

type Result struct {
	Category int   `json:"category"`
	Total    int64 `json:"total"`
	Present  int64 `json:"present"`
	Missing  int64 `json:"missing"`
	collectorapi.DownloadBatchCounts
}

type Preview struct {
	PlanID string `json:"plan_id"`
	Result
}

type plan struct {
	created    time.Time
	preview    Result
	candidates []collectorapi.FavoriteDownloadCandidate
	mu         sync.Mutex
	result     *Result
}

type Service struct {
	libraries *library.SQLiteRepository
	sources   *source.SQLiteRepository
	client    collectorClient
	mu        sync.Mutex
	plans     map[string]*plan
}

func New(db *sql.DB, client collectorClient) *Service {
	return &Service{libraries: library.NewSQLiteRepository(db), sources: source.NewSQLiteRepository(db), client: client, plans: map[string]*plan{}}
}

// Presence deliberately trusts cataloged filename IDs, even for offline roots.
// It neither probes storage nor treats another version as a local replacement.
func (s *Service) present(ctx context.Context) (map[int64]bool, error) {
	libraries, err := s.libraries.List(ctx)
	if err != nil {
		return nil, err
	}
	present := map[int64]bool{}
	for _, lib := range libraries {
		sources, err := s.sources.List(ctx, lib.ID)
		if err != nil {
			return nil, err
		}
		for _, item := range sources {
			name := item.Path
			if item.Kind == source.Directory && name == "." {
				name = filepath.Base(lib.Path)
			}
			if id := source.PandaCandidateID(name, item.Kind); id != 0 {
				present[id] = true
			}
		}
	}
	return present, nil
}

func (s *Service) Preview(ctx context.Context, category int) (Preview, error) {
	if category < 0 || category > 9 {
		return Preview{}, ErrInvalidCategory
	}
	favorites, err := s.client.FavoriteDownloadCandidates(ctx, category)
	if err != nil {
		return Preview{}, err
	}
	present, err := s.present(ctx)
	if err != nil {
		return Preview{}, err
	}
	result := Preview{Result: Result{Category: category, Total: int64(len(favorites))}}
	p := &plan{created: time.Now()}
	for _, favorite := range favorites {
		if present[favorite.Ref.ID] {
			result.Present++
			continue
		}
		result.Missing++
		p.candidates = append(p.candidates, favorite)
		switch favorite.State {
		case "":
			result.NewDownloads++
		case "queued", "running":
			result.ExistingJobs++
		case "completed":
			result.RetainedArchives++
		case "failed":
			result.Failed++
		case "cancelled":
			result.Cancelled++
		case "deleting":
			result.Deleting++
		default:
			return Preview{}, fmt.Errorf("collector returned an invalid download state")
		}
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return Preview{}, err
	}
	result.PlanID = hex.EncodeToString(token[:])
	p.preview = result.Result
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, previous := range s.plans {
		if time.Since(previous.created) > previewLifetime {
			delete(s.plans, id)
		}
	}
	if len(s.plans) >= 16 {
		var oldestID string
		for id, previous := range s.plans {
			if oldestID == "" || previous.created.Before(s.plans[oldestID].created) {
				oldestID = id
			}
		}
		delete(s.plans, oldestID)
	}
	s.plans[result.PlanID] = p
	return result, nil
}

func (s *Service) Execute(ctx context.Context, category int, planID string) (Result, error) {
	if category < 0 || category > 9 {
		return Result{}, ErrInvalidCategory
	}
	s.mu.Lock()
	p, ok := s.plans[planID]
	s.mu.Unlock()
	if !ok || p.preview.Category != category || time.Since(p.created) > previewLifetime {
		return Result{}, ErrInvalidPreview
	}
	// Repeated confirmations share one outcome, including after the response is
	// lost. An uncertain collector response leaves the same scope retryable.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.result != nil {
		return *p.result, nil
	}
	present, err := s.present(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{Category: category, Total: p.preview.Total, Present: p.preview.Present}
	references := make([]panda.GalleryRef, 0, len(p.candidates))
	for _, favorite := range p.candidates {
		if present[favorite.Ref.ID] {
			result.Present++
			continue
		}
		result.Missing++
		// A skipped job may have been deleted since preview. Submitting its
		// reference would recreate it without a fresh review.
		switch favorite.State {
		case "failed":
			result.Failed++
		case "cancelled":
			result.Cancelled++
		case "deleting":
			result.Deleting++
		default:
			references = append(references, favorite.Ref)
		}
	}
	if len(references) > 0 {
		counts, err := s.client.SubmitDownloads(ctx, references)
		if err != nil {
			return Result{}, err
		}
		result.NewDownloads = counts.NewDownloads
		result.ExistingJobs = counts.ExistingJobs
		result.RetainedArchives = counts.RetainedArchives
		result.Failed += counts.Failed
		result.Cancelled += counts.Cancelled
		result.Deleting += counts.Deleting
	}
	p.result = &result
	return result, nil
}
