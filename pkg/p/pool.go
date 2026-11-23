// Package p creates an explicitly owned PostgreSQL connection pool.
// It contains no business models, migrations, global pools, or init-time I/O.
package p

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidConfig      = errors.New("postgres: invalid configuration")
	ErrUnsupportedVersion = errors.New("postgres: server requires PostgreSQL 15 or newer")
)

// Options configures one service's pool. DSN accepts pgx URL or keyword syntax;
// include the host, port, database, user and TLS policy explicitly.
type Options struct {
	DSN string
	// MaxConns defaults to 4; budget the sum of all service/replica pools.
	MaxConns int32
	MinConns int32
	// StartupTimeout defaults to 5 seconds, bounded further by the caller.
	StartupTimeout time.Duration
}

// NewPool returns only after a live PostgreSQL >=15 server has been checked.
// The caller owns the returned pool and must drain requests before Close.
// Transactions and SQL stay in the service's application/outbound adapters.
func NewPool(ctx context.Context, options Options) (*pgxpool.Pool, error) {
	if strings.TrimSpace(options.DSN) == "" || options.MaxConns < 0 ||
		options.MinConns < 0 || options.StartupTimeout < 0 {
		return nil, ErrInvalidConfig
	}
	if options.MaxConns == 0 {
		options.MaxConns = 4
	}
	if options.MinConns > options.MaxConns {
		return nil, ErrInvalidConfig
	}
	config, err := pgxpool.ParseConfig(options.DSN)
	if err != nil {
		// pgx parse errors can include the original DSN and its password.
		return nil, fmt.Errorf("%w: invalid DSN", ErrInvalidConfig)
	}
	config.MaxConns, config.MinConns = options.MaxConns, options.MinConns
	if options.StartupTimeout == 0 {
		options.StartupTimeout = 5 * time.Second
	}
	startCtx, cancel := context.WithTimeout(ctx, options.StartupTimeout)
	defer cancel()
	if err := startCtx.Err(); err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(startCtx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	var version int
	err = pool.QueryRow(startCtx, "select current_setting('server_version_num')::int").Scan(&version)
	if err == nil {
		err = requireVersion(version)
	}
	if err != nil {
		pool.Close()
		if startCtx.Err() != nil {
			return nil, startCtx.Err()
		}
		return nil, fmt.Errorf("postgres: startup check: %w", err)
	}
	return pool, nil
}

func requireVersion(version int) error {
	if version < 150000 {
		return ErrUnsupportedVersion
	}
	return nil
}
