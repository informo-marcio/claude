package store

import (
	"context"
)

// Program é um programa de licenciamento por pontos.
type Program struct {
	Key          string  `json:"key"`
	Manufacturer string  `json:"manufacturer"`
	Name         string  `json:"name"`
	PriceUSD     float64 `json:"priceUsd"`
	Revision     int     `json:"revision"`
}

// Model é um modelo do catálogo com seu consumo diário de pontos.
type Model struct {
	ID                 int64    `json:"id"`
	ProgramKey         string   `json:"programKey"`
	Model              string   `json:"model"`
	Segment            string   `json:"segment"`
	SortOrder          int      `json:"sortOrder"`
	DailyPoints        *float64 `json:"dailyPoints"`
	ServiceDescription string   `json:"serviceDescription"`
	Revision           int      `json:"revision"`
}

// Catalog é o catálogo completo de licenciamento.
type Catalog struct {
	Programs []Program `json:"programs"`
	Models   []Model   `json:"models"`
}

const modelColumns = `id, program_key, model, segment, sort_order, daily_points::float8, service_description, revision`

// Catalog lista programas e modelos.
func (s *Store) Catalog(ctx context.Context) (Catalog, error) {
	c := Catalog{Programs: []Program{}, Models: []Model{}}
	rows, err := s.pool.Query(ctx, `SELECT key, manufacturer, name, price_usd::float8, revision FROM licensing_programs ORDER BY name`)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.Key, &p.Manufacturer, &p.Name, &p.PriceUSD, &p.Revision); err != nil {
			rows.Close()
			return c, err
		}
		c.Programs = append(c.Programs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return c, err
	}
	rows, err = s.pool.Query(ctx, `SELECT `+modelColumns+` FROM licensing_models ORDER BY program_key, sort_order, id`)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.ID, &m.ProgramKey, &m.Model, &m.Segment, &m.SortOrder, &m.DailyPoints, &m.ServiceDescription, &m.Revision); err != nil {
			return c, err
		}
		c.Models = append(c.Models, m)
	}
	return c, rows.Err()
}

// ModelWithProgram devolve um modelo e o preço do ponto do seu programa.
func (s *Store) ModelWithProgram(ctx context.Context, id int64) (Model, Program, error) {
	var m Model
	var p Program
	err := s.pool.QueryRow(ctx, `
		SELECT m.id, m.program_key, m.model, m.segment, m.sort_order, m.daily_points::float8, m.service_description, m.revision,
		       p.key, p.manufacturer, p.name, p.price_usd::float8, p.revision
		FROM licensing_models m JOIN licensing_programs p ON p.key = m.program_key WHERE m.id = $1`, id).
		Scan(&m.ID, &m.ProgramKey, &m.Model, &m.Segment, &m.SortOrder, &m.DailyPoints, &m.ServiceDescription, &m.Revision,
			&p.Key, &p.Manufacturer, &p.Name, &p.PriceUSD, &p.Revision)
	return m, p, notFound(err)
}

// UpdateProgramPrice altera o preço do ponto se a revisão ainda for a lida.
func (s *Store) UpdateProgramPrice(ctx context.Context, key string, priceUSD float64, revision int) (Program, error) {
	var p Program
	err := s.pool.QueryRow(ctx, `UPDATE licensing_programs SET price_usd = $2, revision = revision + 1
		WHERE key = $1 AND revision = $3
		RETURNING key, manufacturer, name, price_usd::float8, revision`, key, priceUSD, revision).
		Scan(&p.Key, &p.Manufacturer, &p.Name, &p.PriceUSD, &p.Revision)
	if notFound(err) == ErrNotFound {
		return p, s.missingOrConflict(ctx, `SELECT EXISTS (SELECT 1 FROM licensing_programs WHERE key = $1)`, key)
	}
	return p, err
}

// UpdateModelConsumption altera consumo e pacote de serviços de um modelo se a
// revisão ainda for a lida. dailyPoints nil volta o modelo a pendente.
func (s *Store) UpdateModelConsumption(ctx context.Context, id int64, dailyPoints *float64, service string, revision int) (Model, error) {
	var m Model
	err := s.pool.QueryRow(ctx, `UPDATE licensing_models SET daily_points = $2, service_description = $3, revision = revision + 1
		WHERE id = $1 AND revision = $4 RETURNING `+modelColumns, id, dailyPoints, service, revision).
		Scan(&m.ID, &m.ProgramKey, &m.Model, &m.Segment, &m.SortOrder, &m.DailyPoints, &m.ServiceDescription, &m.Revision)
	if notFound(err) == ErrNotFound {
		return m, s.missingOrConflict(ctx, `SELECT EXISTS (SELECT 1 FROM licensing_models WHERE id = $1)`, id)
	}
	return m, err
}

// missingOrConflict distingue "não existe" de "revisão desatualizada".
func (s *Store) missingOrConflict(ctx context.Context, existsSQL string, arg any) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, existsSQL, arg).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrConflict
	}
	return ErrNotFound
}
