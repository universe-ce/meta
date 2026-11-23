//go:build integration

package p

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/puddle/v2"
)

func TestLocalPostgresPool(t *testing.T) {
	dsn := os.Getenv("UCE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("integration requires UCE_TEST_POSTGRES_DSN pointing to an isolated local database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, Options{DSN: dsn, MaxConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	t.Run("ready with explicit session identity", func(t *testing.T) {
		var user, database, app string
		var version int
		err := pool.QueryRow(ctx, `select current_user, current_database(),
			current_setting('application_name'), current_setting('server_version_num')::int`).
			Scan(&user, &database, &app, &version)
		if err != nil {
			t.Fatal(err)
		}
		if user != "uce_bus" || database != "uce_bus" || app != "foundation-integration" || version < 150000 {
			t.Fatalf("unexpected session: %s %s %s %d", user, database, app, version)
		}
		t.Logf("local PostgreSQL server_version_num=%d", version)
	})

	t.Run("pool limit respects caller cancellation", func(t *testing.T) {
		one, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer one.Release()
		two, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer two.Release()
		waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		if conn, err := pool.Acquire(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
			if conn != nil {
				conn.Release()
			}
			t.Fatalf("third acquisition should wait for a slot: %v", err)
		}
	})

	t.Run("application transaction commits and rolls back", func(t *testing.T) {
		// TEMP tables are connection scoped; keep one connection for this use case.
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		if _, err := conn.Exec(ctx, "create temp table foundation_tx (value text)"); err != nil {
			t.Fatal(err)
		}
		defer conn.Exec(context.Background(), "drop table foundation_tx")
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into foundation_tx values ($1)", "committed")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		sentinel := errors.New("application rejects this write")
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "insert into foundation_tx values ($1)", "rolled back"); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
		var count int
		var value string
		if err := conn.QueryRow(ctx, "select count(*), min(value) from foundation_tx").Scan(&count, &value); err != nil {
			t.Fatal(err)
		}
		if count != 1 || value != "committed" {
			t.Fatalf("transaction changed application data: %d %q", count, value)
		}
	})

	t.Run("caller owns closing the pool", func(t *testing.T) {
		owned, err := NewPool(ctx, Options{DSN: dsn})
		if err != nil {
			t.Fatal(err)
		}
		owned.Close()
		owned.Close()
		if err := owned.Ping(ctx); err == nil {
			t.Fatal("closed pool still accepts work")
		}
	})

	t.Run("close waits for borrowed connection", func(t *testing.T) {
		owned, err := NewPool(ctx, Options{DSN: dsn, MaxConns: 2})
		if err != nil {
			t.Fatal(err)
		}
		defer owned.Close()
		borrowed, err := owned.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer borrowed.Release()
		spare, err := owned.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		spare.Release()
		started, closed := make(chan struct{}), make(chan struct{})
		go func() {
			close(started)
			owned.Close()
			close(closed)
		}()
		<-started
		acquireCtx, acquireCancel := context.WithTimeout(ctx, 3*time.Second)
		defer acquireCancel()
		// Keep capacity available: a saturated pool waits on its semaphore before
		// checking closure. Actual rejection, not goroutine entry, marks Close.
		for {
			conn, err := owned.Acquire(acquireCtx)
			if conn != nil {
				conn.Release()
			}
			if errors.Is(err, puddle.ErrClosedPool) {
				break
			}
			if err != nil {
				t.Fatalf("closing pool did not reject new work: %v", err)
			}
		}
		select {
		case <-closed:
			t.Fatal("Close returned before the borrowed connection was released")
		default:
		}
		var one int
		if err := borrowed.QueryRow(ctx, "select 1").Scan(&one); err != nil || one != 1 {
			t.Fatalf("Close terminated borrowed work: value=%d error=%v", one, err)
		}
		borrowed.Release()
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("Close did not finish after explicit Release")
		}
	})
}
