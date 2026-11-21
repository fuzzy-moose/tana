package catalog

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidAuthorBonus = errors.New("invalid author bonus")

// SetAuthorBonus sets the weight that the tag of every favorite author weighs
// at least, a number above 0 and at most maxScoreWeight; nil turns it off.
func (s *Service) SetAuthorBonus(ctx context.Context, bonus *float64) error {
	if bonus != nil && (math.IsNaN(*bonus) || *bonus <= 0 || *bonus > maxScoreWeight) {
		return ErrInvalidAuthorBonus
	}
	return s.writeScoreInputs(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE panda_score_settings SET author_bonus = $1 WHERE id = 1", bonus)
		return err
	})
}

// authorBonus reads the author bonus, nil while it is off.
func authorBonus(ctx context.Context, tx pgx.Tx) (*float64, error) {
	var bonus *float64
	err := tx.QueryRow(ctx, "SELECT author_bonus FROM panda_score_settings WHERE id = 1").Scan(&bonus)
	return bonus, err
}
