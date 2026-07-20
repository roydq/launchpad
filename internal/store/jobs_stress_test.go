package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
)

func openStressStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	var (
		db     *sql.DB
		driver Driver
		err    error
	)
	if url := os.Getenv("LAUNCHPAD_TEST_DATABASE_URL"); url != "" {
		db, driver, err = Open(ctx, url)
	} else {
		// Shared in-memory SQLite so concurrent connections see one schema.
		// busy_timeout helps writers retry under lock contention.
		name := "lease-stress-" + uuid.New().String()
		dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", name)
		db, driver, err = Open(ctx, dsn)
		if err == nil {
			db.SetMaxOpenConns(4)
		}
	}
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := Migrate(ctx, db, driver); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	return New(db, driver), db
}

func isBusyErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "locked") || strings.Contains(s, "busy") || strings.Contains(s, "deadlock")
}

func TestLeaseNextConcurrentUnique(t *testing.T) {
	ctx := context.Background()
	s, db := openStressStore(t)
	defer db.Close()

	const nJobs = 16
	const nWorkers = 4
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		for i := 0; i < nJobs; i++ {
			id := uuid.New()
			payload, _ := json.Marshal(map[string]string{"i": id.String()})
			if err := s.EnqueueJob(ctx, tx, &domain.Job{
				Type:         domain.JobTypeDeploy,
				ResourceType: "deployment",
				ResourceID:   id,
				Payload:      payload,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	leased := make(map[uuid.UUID]string)
	var wg sync.WaitGroup
	errCh := make(chan error, nWorkers)
	for w := 0; w < nWorkers; w++ {
		wg.Add(1)
		workerID := fmt.Sprintf("w-%d", w)
		go func(wid string) {
			defer wg.Done()
			for {
				var job *domain.Job
				var err error
				for attempt := 0; attempt < 80; attempt++ {
					job, err = s.LeaseNext(ctx, wid, []domain.JobType{domain.JobTypeDeploy}, time.Minute)
					if err == nil || !isBusyErr(err) {
						break
					}
					time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				}
				if err != nil {
					errCh <- err
					return
				}
				if job == nil {
					return
				}
				mu.Lock()
				if prev, ok := leased[job.ID]; ok {
					errCh <- fmt.Errorf("job %s leased by %s and %s", job.ID, prev, wid)
					mu.Unlock()
					return
				}
				leased[job.ID] = wid
				mu.Unlock()
			}
		}(workerID)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(leased) != nJobs {
		t.Fatalf("leased %d want %d", len(leased), nJobs)
	}
}

func TestReclaimExpiredLeaseThenRelease(t *testing.T) {
	ctx := context.Background()
	db, driver, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	s := New(db, driver)

	resourceID := uuid.New()
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		return s.EnqueueJob(ctx, tx, &domain.Job{
			Type:         domain.JobTypeDeploy,
			ResourceType: "deployment",
			ResourceID:   resourceID,
			Payload:      json.RawMessage(`{}`),
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := s.LeaseNext(ctx, "worker-a", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil || job == nil {
		t.Fatalf("first lease: %v %#v", err, job)
	}

	// SQLite formatTime is second-precision; force lease into the past.
	past := formatTime(driver, time.Now().UTC().Add(-2*time.Second))
	_, err = db.ExecContext(ctx, s.q(`UPDATE jobs SET leased_until = ? WHERE id = ?`), past, job.ID.String())
	if err != nil {
		t.Fatal(err)
	}

	n, err := s.ReclaimExpiredLeases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected reclaim, got %d", n)
	}

	job2, err := s.LeaseNext(ctx, "worker-b", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil || job2 == nil {
		t.Fatalf("second lease: %v %#v", err, job2)
	}
	if job2.ID != job.ID {
		t.Fatalf("want same job %s got %s", job.ID, job2.ID)
	}
	if job2.LeasedBy != "worker-b" {
		t.Fatalf("leased_by %q", job2.LeasedBy)
	}
}
