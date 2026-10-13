package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgDateFromTime(t time.Time) pgtype.Date {
	utc := t.UTC()
	d := pgtype.Date{}
	_ = d.Scan(utc.Format("2006-01-02"))
	return d
}

func timeFromPg(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
