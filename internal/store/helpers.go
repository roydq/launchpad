package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) exec(tx *sql.Tx) execer {
	if tx != nil {
		return tx
	}
	return s.db
}

func (s *Store) q(query string) string {
	return rebind(s.driver, query)
}

func formatTime(driver Driver, t time.Time) string {
	if driver == DriverPostgres {
		return t.Format(time.RFC3339Nano)
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func parseTime(driver Driver, value string) time.Time {
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"}
	for _, f := range formats {
		if t, err := time.Parse(f, value); err == nil {
			return t
		}
	}
	_ = driver
	return time.Time{}
}

func rebind(driver Driver, query string) string {
	if driver != DriverPostgres {
		return query
	}
	n := 1
	var out strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			fmt.Fprintf(&out, "$%d", n)
			n++
		} else {
			out.WriteByte(query[i])
		}
	}
	return out.String()
}