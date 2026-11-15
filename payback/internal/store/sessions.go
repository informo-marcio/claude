package store

import (
	"context"
	"time"

	"github.com/informo-marcio/claude/payback/internal/auth"
)

// CreateSession grava uma sessão nova e aproveita para limpar as vencidas.
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, id auth.Identity, expiresAt time.Time) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_hash, subject, email, name, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, tokenHash, id.Subject, id.Email, id.Name, expiresAt)
	return err
}

// SessionIdentity devolve a identidade de uma sessão ainda válida.
func (s *Store) SessionIdentity(ctx context.Context, tokenHash []byte) (auth.Identity, error) {
	var id auth.Identity
	err := s.pool.QueryRow(ctx, `SELECT subject, email, name FROM sessions
		WHERE token_hash = $1 AND expires_at > now()`, tokenHash).Scan(&id.Subject, &id.Email, &id.Name)
	return id, notFound(err)
}

// DeleteSession encerra uma sessão; não existir não é erro.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}
