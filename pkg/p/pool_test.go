package p

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRejectInvalidOptionsWithoutExposingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"empty DSN", Options{}},
		{"malformed DSN", Options{DSN: "postgres://user:private-password@localhost:bad/db"}},
		{"negative maximum", Options{DSN: "postgres://localhost/test", MaxConns: -1}},
		{"negative minimum", Options{DSN: "postgres://localhost/test", MinConns: -1}},
		{"minimum exceeds maximum", Options{DSN: "postgres://localhost/test", MaxConns: 2, MinConns: 3}},
		{"negative startup timeout", Options{DSN: "postgres://localhost/test", StartupTimeout: -time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := NewPool(context.Background(), tc.opts)
			if pool != nil {
				pool.Close()
				t.Fatal("invalid options returned a usable pool")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("want ErrInvalidConfig, got %v", err)
			}
			if strings.Contains(err.Error(), "private-password") {
				t.Fatal("configuration error exposed credentials")
			}
		})
	}
}

func TestCanceledStartupReturnsNoPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool, err := NewPool(ctx, Options{DSN: "postgres://localhost/test?sslmode=disable"})
	if pool != nil {
		pool.Close()
		t.Fatal("canceled startup returned a pool")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled context, got %v", err)
	}
}

func TestStartupDeadlineBoundsStalledServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
		close(accepted)
	}()
	start := time.Now()
	pool, err := NewPool(context.Background(), Options{
		DSN:            "postgres://test:test@" + listener.Addr().String() + "/test?sslmode=disable",
		StartupTimeout: 100 * time.Millisecond,
	})
	if pool != nil {
		pool.Close()
		t.Fatal("server that never responds returned a pool")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want startup deadline, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("startup ignored its time limit")
	}
	listener.Close()
	for conn := range accepted {
		conn.Close()
	}
}

func TestCallerDeadlineBoundsStalledStartup(t *testing.T) {
	listener, connected, peersClosed := stalledPostgresHandshake(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	type result struct {
		poolReturned bool
		err          error
	}
	returned := make(chan result, 1)
	callDone := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		select {
		case <-callDone:
		case <-time.After(6 * time.Second):
			t.Error("startup caller did not exit")
		}
	})
	start := time.Now()
	go func() {
		defer close(callDone)
		pool, err := NewPool(ctx, Options{
			DSN:            "postgres://test:test@" + listener.Addr().String() + "/test?sslmode=disable",
			StartupTimeout: 5 * time.Second,
		})
		if pool != nil {
			pool.Close()
		}
		returned <- result{poolReturned: pool != nil, err: err}
	}()
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("startup did not reach a live TCP handshake")
	}
	select {
	case got := <-returned:
		if got.poolReturned || !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("short caller deadline: pool=%t error=%v", got.poolReturned, got.err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second || ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("startup ignored caller deadline: elapsed=%s context=%v", elapsed, ctx.Err())
		}
	case <-time.After(6 * time.Second):
		t.Fatal("startup remained blocked beyond both deadlines")
	}
	peersClosed()
}

func stalledPostgresHandshake(t *testing.T) (net.Listener, <-chan struct{}, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	connected, accepted := make(chan struct{}), make(chan struct{})
	readResults := make(chan error, 16)
	var once sync.Once
	var readers sync.WaitGroup
	var connections []net.Conn // Only read after the accept loop has exited.
	go func() {
		defer close(accepted)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections = append(connections, conn)
			readers.Add(1)
			go func() {
				defer readers.Done()
				defer conn.Close()
				var first [1]byte
				_, err := io.ReadFull(conn, first[:])
				if err == nil {
					once.Do(func() { close(connected) })
					_, err = io.Copy(io.Discard, conn)
				}
				readResults <- err
			}()
		}
	}()
	stop := func() {
		_ = listener.Close()
		<-accepted
	}
	t.Cleanup(func() {
		stop()
		for _, conn := range connections {
			_ = conn.Close()
		}
		done := make(chan struct{})
		go func() { readers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("stalled handshake fixture readers did not exit")
		}
	})
	return listener, connected, func() {
		stop()
		for range connections {
			select {
			case err := <-readResults:
				if err != nil {
					t.Errorf("startup peer did not close cleanly: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("startup returned with a TCP connection still open")
			}
		}
	}
}

func TestMinimumSupportedServerVersion(t *testing.T) {
	for _, tc := range []struct {
		version int
		ok      bool
	}{
		{0, false},
		{140020, false},
		{149999, false},
		{150000, true},
		{150018, true},
		{170000, true},
	} {
		if err := requireVersion(tc.version); tc.ok && err != nil {
			t.Errorf("%d should be supported: %v", tc.version, err)
		} else if !tc.ok && !errors.Is(err, ErrUnsupportedVersion) {
			t.Errorf("%d should be rejected: %v", tc.version, err)
		}
	}
}
