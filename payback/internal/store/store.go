// Package store é a camada de dados do portal sobre PostgreSQL.
//
// Toda operação de escrita com mais de um comando roda em transação. O pacote
// não decide permissões: quem chama (internal/api) autoriza antes.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var (
	// ErrNotFound indica registro inexistente.
	ErrNotFound = errors.New("registro não encontrado")
	// ErrConflict indica que o registro mudou desde a leitura.
	ErrConflict = errors.New("o registro foi alterado por outra pessoa; recarregue e tente de novo")
)

// Store dá acesso ao banco.
type Store struct {
	pool *pgxpool.Pool
}

// Open conecta ao banco e confirma que ele responde.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("conexão com o banco: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("banco não respondeu: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close encerra as conexões.
func (s *Store) Close() { s.pool.Close() }

// Ping confere se o banco responde; usado pela verificação de saúde.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate aplica, em ordem e uma única vez, as migrações embutidas.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		sql, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		err = s.tx(ctx, func(tx pgx.Tx) error {
			// Trava para que duas instâncias não migrem ao mesmo tempo.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7241001)`); err != nil {
				return err
			}
			var done bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&done); err != nil {
				return err
			}
			if done {
				return nil
			}
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return fmt.Errorf("migração %s: %w", name, err)
		}
	}
	return nil
}

// tx roda fn em uma transação, confirmando só se fn não devolver erro.
func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sem efeito após Commit
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
