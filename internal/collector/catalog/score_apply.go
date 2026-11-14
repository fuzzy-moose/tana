package catalog

import (
	"context"
	"errors"
)

var (
	ErrScoreCandidateNotFound = errors.New("score candidate not found")
	ErrInvalidScoreThreshold  = errors.New("invalid score threshold")
	ErrNoActiveScoreModel     = errors.New("no active score model")
)

// The range of a threshold in tenths.
const (
	minScoreThreshold = -1000
	maxScoreThreshold = 1000
)

// ApplyScoreModel makes the stored candidate the active model with the
// threshold, and deletes the model that was active.
func (s *Service) ApplyScoreModel(ctx context.Context, candidateID int64, thresholdTenths int) error {
	if thresholdTenths < minScoreThreshold || thresholdTenths > maxScoreThreshold {
		return ErrInvalidScoreThreshold
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Lock before reading the candidate (ADR 0018): applies run one after
	// another, and a preview that has deleted the candidate commits first. Row
	// locks can't do this: the first apply has no active row to lock.
	if _, err := tx.Exec(ctx, "LOCK TABLE panda_score_models IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return err
	}
	var found bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM panda_score_models WHERE id = $1 AND NOT active)", candidateID).Scan(&found); err != nil {
		return err
	}
	if !found {
		return ErrScoreCandidateNotFound
	}
	if _, err := tx.Exec(ctx, "DELETE FROM panda_score_models WHERE active"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE panda_score_models SET active = true, threshold_tenths = $2 WHERE id = $1", candidateID, thresholdTenths); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetScoreThreshold changes the threshold of the active model.
func (s *Service) SetScoreThreshold(ctx context.Context, thresholdTenths int) error {
	if thresholdTenths < minScoreThreshold || thresholdTenths > maxScoreThreshold {
		return ErrInvalidScoreThreshold
	}
	result, err := s.db.Exec(ctx, "UPDATE panda_score_models SET threshold_tenths = $1 WHERE active", thresholdTenths)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNoActiveScoreModel
	}
	return nil
}

// RemoveScoreModel deletes the active model, if there is one. Candidates and
// overrides stay.
func (s *Service) RemoveScoreModel(ctx context.Context) error {
	_, err := s.db.Exec(ctx, "DELETE FROM panda_score_models WHERE active")
	return err
}
