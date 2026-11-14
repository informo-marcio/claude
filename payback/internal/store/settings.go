package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/informo-marcio/claude/payback/internal/finance"
)

// Policy é o conteúdo de uma versão da política financeira.
type Policy struct {
	Settings finance.Settings `json:"settings"`
	// InternalICMS são as alíquotas internas por UF usadas no DIFAL.
	InternalICMS map[string]float64 `json:"internalIcms"`
}

// Validate confere a política antes de gravar.
func (p Policy) Validate() error {
	if err := p.Settings.Validate(); err != nil {
		return err
	}
	if len(p.InternalICMS) == 0 {
		return errors.New("informe as alíquotas internas de ICMS")
	}
	for uf, rate := range p.InternalICMS {
		if len(uf) != 2 || rate < 0 || rate > 1 {
			return errors.New("alíquota interna de ICMS inválida para " + uf)
		}
	}
	return nil
}

// DefaultPolicy é a política carregada em um banco novo.
func DefaultPolicy() Policy {
	return Policy{Settings: finance.CurrentPolicy(), InternalICMS: finance.DefaultInternalICMS()}
}

// SettingsVersion é uma versão gravada da política.
type SettingsVersion struct {
	ID        int64     `json:"id"`
	Policy    Policy    `json:"policy"`
	CreatedAt time.Time `json:"createdAt"`
}

func scanSettings(row pgx.Row) (SettingsVersion, error) {
	var v SettingsVersion
	var payload []byte
	if err := row.Scan(&v.ID, &payload, &v.CreatedAt); err != nil {
		return v, notFound(err)
	}
	return v, json.Unmarshal(payload, &v.Policy)
}

// SeedSettings grava a política padrão se o banco ainda não tiver nenhuma.
func (s *Store) SeedSettings(ctx context.Context) error {
	payload, err := json.Marshal(DefaultPolicy())
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO settings_versions (payload)
		SELECT $1 WHERE NOT EXISTS (SELECT 1 FROM settings_versions)`, payload)
	return err
}

// CurrentSettings devolve a política vigente.
func (s *Store) CurrentSettings(ctx context.Context) (SettingsVersion, error) {
	return scanSettings(s.pool.QueryRow(ctx,
		`SELECT id, payload, created_at FROM settings_versions ORDER BY id DESC LIMIT 1`))
}

// SettingsByIDs devolve as versões pedidas, indexadas pelo id.
func (s *Store) SettingsByIDs(ctx context.Context, ids []int64) (map[int64]SettingsVersion, error) {
	out := map[int64]SettingsVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id, payload, created_at FROM settings_versions WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanSettings(rows)
		if err != nil {
			return nil, err
		}
		out[v.ID] = v
	}
	return out, rows.Err()
}

// SaveSettings grava uma nova versão da política. baseID é a versão que o
// administrador estava editando; se já houver outra mais nova, a gravação é
// recusada para não sobrescrever a alteração de outra pessoa.
func (s *Store) SaveSettings(ctx context.Context, baseID int64, p Policy, userID int64) (SettingsVersion, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return SettingsVersion{}, err
	}
	var out SettingsVersion
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `LOCK TABLE settings_versions IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		var current int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM settings_versions`).Scan(&current); err != nil {
			return err
		}
		if current != baseID {
			return ErrConflict
		}
		out, err = scanSettings(tx.QueryRow(ctx, `INSERT INTO settings_versions (payload, created_by)
			VALUES ($1, $2) RETURNING id, payload, created_at`, payload, userID))
		return err
	})
	return out, err
}
