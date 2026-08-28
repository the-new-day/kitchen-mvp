// Package postgres implements the repositories of the platform on top of pgx.
package postgres

import (
	"avito-kitchen/internal/config"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the connection pool of the platform database
// and the entry point of every repository.
type DB struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// New opens the pool and verifies that the database answers.
func New(ctx context.Context, cfg config.Postgres, log *slog.Logger) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.InfoContext(ctx, "postgres pool ready", slog.Int("max_conns", int(cfg.MaxConns)))

	return &DB{pool: pool, log: log}, nil
}

// Close waits for the borrowed connections and releases the pool.
func (db *DB) Close() {
	db.pool.Close()
}

// queryer is the part of pgx that both the pool and a transaction implement.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type txKey struct{}

// InTx runs fn inside a transaction and passes it a context carrying that
// transaction, so that the repositories called from fn join it. A nested call
// reuses the transaction already in the context instead of opening a second one.
func (db *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		err := tx.Rollback(context.WithoutCancel(ctx))
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			db.log.ErrorContext(ctx, "rollback failed", slog.String("error", err.Error()))
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// conn returns the transaction of the context, or the pool when there is none.
func (db *DB) conn(ctx context.Context) queryer {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return db.pool
}
