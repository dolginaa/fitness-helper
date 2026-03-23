package postgres

import (
	"errors"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/jackc/pgx/v5"
)

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
