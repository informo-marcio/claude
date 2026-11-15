package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Status operacional de um projeto.
const (
	StatusDraft     = "draft"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// Project é uma proposta ou contrato de locação.
type Project struct {
	ID                    int64     `json:"id"`
	OwnerID               int64     `json:"ownerId"`
	OwnerName             string    `json:"ownerName"`
	ClientName            string    `json:"clientName"`
	Name                  string    `json:"name"`
	Status                string    `json:"status"`
	IsTest                bool      `json:"isTest"`
	DurationMonths        int       `json:"durationMonths"`
	MonthlyRevenue        float64   `json:"monthlyRevenue"`
	ImplementationRevenue float64   `json:"implementationRevenue"`
	ExpectedLossRate      float64   `json:"expectedLossRate"`
	MonthlySupportCost    float64   `json:"monthlySupportCost"`
	ExchangeRate          *float64  `json:"exchangeRate"`
	DestState             string    `json:"destState"`
	ContractStartDate     *string   `json:"contractStartDate"`
	DealID                *int64    `json:"dealId"`
	Notes                 string    `json:"notes"`
	SettingsVersionID     *int64    `json:"settingsVersionId"`
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

// License são as premissas congeladas de um item de licença por pontos.
type License struct {
	ProgramKey         string  `json:"programKey"`
	ModelID            int64   `json:"modelId"`
	ModelName          string  `json:"modelName"`
	DailyPoints        float64 `json:"dailyPoints"`
	PriceUSD           float64 `json:"priceUsd"`
	ServiceDescription string  `json:"serviceDescription"`
}

// Item é um item de custo de um projeto.
type Item struct {
	ID          int64    `json:"id"`
	ProjectID   int64    `json:"projectId"`
	Position    int      `json:"position"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Quantity    int      `json:"quantity"`
	UnitPrice   float64  `json:"unitPrice"`
	Currency    string   `json:"currency"`
	OriginState *string  `json:"originState"`
	Imported    bool     `json:"imported"`
	License     *License `json:"license"`
}

// Event é um registro da trilha de auditoria.
type Event struct {
	ID        int64  `json:"id"`
	UserName  string `json:"userName"`
	UserEmail string `json:"userEmail"`
	// AsAdmin indica alteração feita por administrador em projeto alheio.
	AsAdmin      bool      `json:"asAdmin"`
	ProjectID    *int64    `json:"projectId"`
	ProjectLabel *string   `json:"projectLabel"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Actor é quem está fazendo uma alteração, para a trilha de auditoria.
type Actor struct {
	UserID int64
	Email  string
	Name   string
	// AsAdmin é verdadeiro quando o administrador age sobre projeto alheio.
	AsAdmin bool
}

// Share é uma concessão de leitura de um projeto.
type Share struct {
	UserID int64  `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

const projectColumns = `p.id, p.owner_id, u.name, p.client_name, p.name, p.status, p.is_test, p.duration_months,
	p.monthly_revenue::float8, p.implementation_revenue::float8, p.expected_loss_rate::float8,
	p.monthly_support_cost::float8, p.exchange_rate::float8, p.dest_state,
	to_char(p.contract_start_date, 'YYYY-MM-DD'), p.deal_id, p.notes, p.settings_version_id,
	p.version, p.created_at, p.updated_at`

const projectFrom = ` FROM projects p JOIN users u ON u.id = p.owner_id `

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.OwnerID, &p.OwnerName, &p.ClientName, &p.Name, &p.Status, &p.IsTest, &p.DurationMonths,
		&p.MonthlyRevenue, &p.ImplementationRevenue, &p.ExpectedLossRate, &p.MonthlySupportCost,
		&p.ExchangeRate, &p.DestState, &p.ContractStartDate, &p.DealID, &p.Notes, &p.SettingsVersionID,
		&p.Version, &p.CreatedAt, &p.UpdatedAt)
	return p, notFound(err)
}

// audit grava um registro na trilha, na mesma transação da alteração. Com
// projectID, copia o rótulo do projeto como ele está naquele momento.
func audit(ctx context.Context, tx pgx.Tx, a Actor, projectID *int64, description string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log (user_id, user_email, user_name, as_admin, project_id, project_label, description)
		VALUES ($1, $2, $3, $4, $5, (SELECT client_name || ' — ' || name FROM projects WHERE id = $5), $6)`,
		a.UserID, a.Email, a.Name, a.AsAdmin, projectID, description)
	return err
}

// Project busca um projeto pelo id.
func (s *Store) Project(ctx context.Context, id int64) (Project, error) {
	return scanProject(s.pool.QueryRow(ctx, `SELECT `+projectColumns+projectFrom+`WHERE p.id = $1`, id))
}

// ProjectsVisibleTo lista os projetos que o usuário pode ler: os próprios, os
// compartilhados com ele e, se all for verdadeiro, todos.
func (s *Store) ProjectsVisibleTo(ctx context.Context, userID int64, all bool) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+projectColumns+projectFrom+`
		WHERE $2 OR p.owner_id = $1
		   OR EXISTS (SELECT 1 FROM project_shares s WHERE s.project_id = p.id AND s.user_id = $1)
		ORDER BY p.updated_at DESC, p.id DESC`, userID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// IsSharedWith informa se o projeto foi compartilhado com o usuário.
func (s *Store) IsSharedWith(ctx context.Context, projectID, userID int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_shares WHERE project_id = $1 AND user_id = $2)`,
		projectID, userID).Scan(&ok)
	return ok, err
}

// CreateProject grava um projeto novo e o primeiro evento do histórico.
func (s *Store) CreateProject(ctx context.Context, p Project, a Actor) (Project, error) {
	var id int64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO projects (owner_id, client_name, name, status, is_test, duration_months,
				monthly_revenue, implementation_revenue, expected_loss_rate, monthly_support_cost, exchange_rate,
				dest_state, contract_start_date, deal_id, notes, settings_version_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::date, $14, $15, $16) RETURNING id`,
			a.UserID, p.ClientName, p.Name, p.Status, p.IsTest, p.DurationMonths, p.MonthlyRevenue,
			p.ImplementationRevenue, p.ExpectedLossRate, p.MonthlySupportCost, p.ExchangeRate, p.DestState,
			p.ContractStartDate, p.DealID, p.Notes, p.SettingsVersionID).Scan(&id)
		if err != nil {
			return err
		}
		return audit(ctx, tx, a, &id, "Projeto criado")
	})
	if err != nil {
		return Project{}, err
	}
	return s.Project(ctx, id)
}

// UpdateProject grava as alterações se a versão ainda for a que o usuário leu.
func (s *Store) UpdateProject(ctx context.Context, p Project, a Actor, description string) (Project, error) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE projects SET client_name = $3, name = $4, status = $5, is_test = $6,
				duration_months = $7, monthly_revenue = $8, implementation_revenue = $9, expected_loss_rate = $10,
				monthly_support_cost = $11, exchange_rate = $12, dest_state = $13, contract_start_date = $14::date,
				deal_id = $15, notes = $16, settings_version_id = $17, version = version + 1, updated_at = now()
			WHERE id = $1 AND version = $2`,
			p.ID, p.Version, p.ClientName, p.Name, p.Status, p.IsTest, p.DurationMonths, p.MonthlyRevenue,
			p.ImplementationRevenue, p.ExpectedLossRate, p.MonthlySupportCost, p.ExchangeRate, p.DestState,
			p.ContractStartDate, p.DealID, p.Notes, p.SettingsVersionID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrConflict
		}
		return audit(ctx, tx, a, &p.ID, description)
	})
	if err != nil {
		return Project{}, err
	}
	return s.Project(ctx, p.ID)
}

// DeleteProject apaga o projeto; itens e compartilhamentos caem em cascata. A
// exclusão fica registrada na trilha de auditoria, que não é apagada.
func (s *Store) DeleteProject(ctx context.Context, id int64, a Actor) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := audit(ctx, tx, a, &id, "Projeto excluído"); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

const itemColumns = `id, project_id, position, name, kind, quantity, unit_price::float8, currency, origin_state, imported,
	lic_program_key, lic_model_id, lic_model_name, lic_daily_points::float8, lic_price_usd::float8, lic_service_description`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	var progKey, modelName, service *string
	var modelID *int64
	var daily, price *float64
	err := row.Scan(&it.ID, &it.ProjectID, &it.Position, &it.Name, &it.Kind, &it.Quantity, &it.UnitPrice, &it.Currency,
		&it.OriginState, &it.Imported, &progKey, &modelID, &modelName, &daily, &price, &service)
	if err != nil {
		return it, notFound(err)
	}
	if progKey != nil && modelName != nil && daily != nil && price != nil {
		it.License = &License{ProgramKey: *progKey, ModelName: *modelName, DailyPoints: *daily, PriceUSD: *price}
		if modelID != nil {
			it.License.ModelID = *modelID
		}
		if service != nil {
			it.License.ServiceDescription = *service
		}
	}
	return it, nil
}

// ItemsByProjects devolve os itens dos projetos pedidos, agrupados por projeto.
func (s *Store) ItemsByProjects(ctx context.Context, projectIDs []int64) (map[int64][]Item, error) {
	out := map[int64][]Item{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+itemColumns+` FROM project_items
		WHERE project_id = ANY($1) ORDER BY project_id, position, id`, projectIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out[it.ProjectID] = append(out[it.ProjectID], it)
	}
	return out, rows.Err()
}

// Items devolve os itens de um projeto, em ordem.
func (s *Store) Items(ctx context.Context, projectID int64) ([]Item, error) {
	m, err := s.ItemsByProjects(ctx, []int64{projectID})
	if err != nil {
		return nil, err
	}
	if m[projectID] == nil {
		return []Item{}, nil
	}
	return m[projectID], nil
}

func licenseArgs(l *License) []any {
	if l == nil {
		return []any{nil, nil, nil, nil, nil, nil}
	}
	return []any{l.ProgramKey, l.ModelID, l.ModelName, l.DailyPoints, l.PriceUSD, l.ServiceDescription}
}

func touchProject(ctx context.Context, tx pgx.Tx, projectID int64) error {
	_, err := tx.Exec(ctx, `UPDATE projects SET version = version + 1, updated_at = now() WHERE id = $1`, projectID)
	return err
}

// CreateItem inclui um item ao fim da lista do projeto.
func (s *Store) CreateItem(ctx context.Context, it Item, a Actor, description string) (Item, error) {
	var out Item
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := append([]any{it.ProjectID, it.Name, it.Kind, it.Quantity, it.UnitPrice, it.Currency, it.OriginState, it.Imported},
			licenseArgs(it.License)...)
		var err error
		out, err = scanItem(tx.QueryRow(ctx, `INSERT INTO project_items (project_id, position, name, kind, quantity,
				unit_price, currency, origin_state, imported, lic_program_key, lic_model_id, lic_model_name,
				lic_daily_points, lic_price_usd, lic_service_description)
			VALUES ($1, (SELECT COALESCE(max(position), 0) + 1 FROM project_items WHERE project_id = $1),
				$2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING `+itemColumns, args...))
		if err != nil {
			return err
		}
		if err := touchProject(ctx, tx, it.ProjectID); err != nil {
			return err
		}
		return audit(ctx, tx, a, &it.ProjectID, description)
	})
	return out, err
}

// UpdateItem altera um item; o item precisa pertencer ao projeto informado.
func (s *Store) UpdateItem(ctx context.Context, it Item, a Actor, description string) (Item, error) {
	var out Item
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := append([]any{it.ID, it.ProjectID, it.Name, it.Kind, it.Quantity, it.UnitPrice, it.Currency, it.OriginState, it.Imported},
			licenseArgs(it.License)...)
		var err error
		out, err = scanItem(tx.QueryRow(ctx, `UPDATE project_items SET name = $3, kind = $4, quantity = $5,
				unit_price = $6, currency = $7, origin_state = $8, imported = $9, lic_program_key = $10,
				lic_model_id = $11, lic_model_name = $12, lic_daily_points = $13, lic_price_usd = $14,
				lic_service_description = $15
			WHERE id = $1 AND project_id = $2 RETURNING `+itemColumns, args...))
		if err != nil {
			return err
		}
		if err := touchProject(ctx, tx, it.ProjectID); err != nil {
			return err
		}
		return audit(ctx, tx, a, &it.ProjectID, description)
	})
	return out, err
}

// Item busca um item de um projeto.
func (s *Store) Item(ctx context.Context, projectID, itemID int64) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `SELECT `+itemColumns+` FROM project_items WHERE id = $1 AND project_id = $2`,
		itemID, projectID))
}

// DeleteItem remove um item do projeto.
func (s *Store) DeleteItem(ctx context.Context, projectID, itemID int64, a Actor, description string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM project_items WHERE id = $1 AND project_id = $2`, itemID, projectID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := touchProject(ctx, tx, projectID); err != nil {
			return err
		}
		return audit(ctx, tx, a, &projectID, description)
	})
}

// DuplicateItem copia um item e posiciona a cópia logo abaixo do original.
func (s *Store) DuplicateItem(ctx context.Context, projectID, itemID int64, a Actor) (Item, error) {
	var out Item
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Trava o projeto para que duas duplicações não disputem posições.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM projects WHERE id = $1 FOR UPDATE`, projectID); err != nil {
			return err
		}
		var position int
		if err := tx.QueryRow(ctx, `SELECT position FROM project_items WHERE id = $1 AND project_id = $2`,
			itemID, projectID).Scan(&position); err != nil {
			return notFound(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE project_items SET position = position + 1 WHERE project_id = $1 AND position > $2`,
			projectID, position); err != nil {
			return err
		}
		var err error
		out, err = scanItem(tx.QueryRow(ctx, `INSERT INTO project_items (project_id, position, name, kind, quantity,
				unit_price, currency, origin_state, imported, lic_program_key, lic_model_id, lic_model_name,
				lic_daily_points, lic_price_usd, lic_service_description)
			SELECT project_id, position + 1, name || ' (cópia)', kind, quantity, unit_price, currency, origin_state,
				imported, lic_program_key, lic_model_id, lic_model_name, lic_daily_points, lic_price_usd,
				lic_service_description
			FROM project_items WHERE id = $1 RETURNING `+itemColumns, itemID))
		if err != nil {
			return err
		}
		if err := touchProject(ctx, tx, projectID); err != nil {
			return err
		}
		return audit(ctx, tx, a, &projectID, "Item duplicado: "+out.Name)
	})
	return out, err
}

const eventColumns = `id, user_name, user_email, as_admin, project_id, project_label, description, created_at`

func (s *Store) events(ctx context.Context, sql string, args ...any) ([]Event, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.UserName, &e.UserEmail, &e.AsAdmin, &e.ProjectID, &e.ProjectLabel, &e.Description, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Events devolve o histórico de um projeto, do mais recente ao mais antigo.
func (s *Store) Events(ctx context.Context, projectID int64) ([]Event, error) {
	return s.events(ctx, `SELECT `+eventColumns+` FROM audit_log WHERE project_id = $1 ORDER BY id DESC LIMIT 500`, projectID)
}

// AuditLog devolve a trilha completa, inclusive de projetos excluídos e de
// alterações de política e catálogo. beforeID pagina para trás; zero começa
// do registro mais recente.
func (s *Store) AuditLog(ctx context.Context, beforeID int64, limit int) ([]Event, error) {
	return s.events(ctx, `SELECT `+eventColumns+` FROM audit_log
		WHERE $1 = 0 OR id < $1 ORDER BY id DESC LIMIT $2`, beforeID, limit)
}

// Shares lista com quem o projeto está compartilhado.
func (s *Store) Shares(ctx context.Context, projectID int64) ([]Share, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.email, u.name FROM project_shares s JOIN users u ON u.id = s.user_id
		WHERE s.project_id = $1 ORDER BY u.name, u.email`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Share{}
	for rows.Next() {
		var sh Share
		if err := rows.Scan(&sh.UserID, &sh.Email, &sh.Name); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// AddShare concede leitura do projeto a um usuário; repetir não é erro.
func (s *Store) AddShare(ctx context.Context, projectID int64, target User, a Actor) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO project_shares (project_id, user_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, projectID, target.ID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return audit(ctx, tx, a, &projectID, "Projeto compartilhado com "+target.Email)
	})
}

// RemoveShare retira a concessão de leitura.
func (s *Store) RemoveShare(ctx context.Context, projectID, userID int64, a Actor) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(ctx, `DELETE FROM project_shares s USING users u
			WHERE s.project_id = $1 AND s.user_id = $2 AND u.id = s.user_id RETURNING u.email`, projectID, userID).Scan(&email)
		if err != nil {
			return notFound(err)
		}
		return audit(ctx, tx, a, &projectID, "Compartilhamento removido de "+email)
	})
}
