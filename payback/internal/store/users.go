package store

import (
	"context"
	"strings"
	"time"
)

// Papéis de usuário.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// User é uma pessoa autenticada no portal.
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// IsAdmin informa se o usuário administra política e catálogo.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

const userColumns = `id, email, name, role, created_at`

// UpsertUser registra ou atualiza quem acabou de se autenticar. O vínculo é
// pelo identificador estável do provedor quando existe e, senão, pelo e-mail.
// makeAdmin promove o usuário; nunca rebaixa um administrador existente.
func (s *Store) UpsertUser(ctx context.Context, subject, email, name string, makeAdmin bool) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	role := RoleUser
	if makeAdmin {
		role = RoleAdmin
	}
	var sub *string
	if subject != "" {
		sub = &subject
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, subject, role) VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET
			name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE users.name END,
			subject = COALESCE(users.subject, EXCLUDED.subject),
			role = CASE WHEN EXCLUDED.role = 'admin' THEN 'admin' ELSE users.role END,
			last_seen_at = now()
		RETURNING `+userColumns, email, name, sub, role).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	return u, err
}

// UserByEmail busca um usuário pelo e-mail exato.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(email))).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	return u, notFound(err)
}
