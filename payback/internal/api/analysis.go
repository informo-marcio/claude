package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/informo-marcio/claude/payback/internal/finance"
	"github.com/informo-marcio/claude/payback/internal/store"
)

// Analysis é o resultado financeiro de um projeto, sempre calculado na hora a
// partir dos itens e da política; nada é guardado, então nunca fica defasado.
type Analysis struct {
	// Status é "ready" ou "blocked". Bloqueado significa que faltam premissas;
	// nenhum número é exibido nesse caso.
	Status            string                   `json:"status"`
	Blockers          []string                 `json:"blockers,omitempty"`
	SettingsVersionID int64                    `json:"settingsVersionId"`
	Capex             *finance.Capex           `json:"capex,omitempty"`
	Licensing         *finance.PointsLicensing `json:"licensing,omitempty"`
	Scenarios         *finance.Scenarios       `json:"scenarios,omitempty"`
	Health            *finance.Health          `json:"health,omitempty"`
	IRR               *finance.IRRAssessment   `json:"irr,omitempty"`
	// Sensitivity mostra o cenário pessimista com a mensalidade variando.
	Sensitivity []SensitivityRow `json:"sensitivity,omitempty"`
}

// SensitivityRow é o resultado com a mensalidade alterada em Delta.
type SensitivityRow struct {
	// Delta é a variação da mensalidade como fração (-0.1 = 10% menor).
	Delta          float64           `json:"delta"`
	MonthlyRevenue float64           `json:"monthlyRevenue"`
	TotalRevenue   float64           `json:"totalRevenue"`
	NPV            float64           `json:"npv"`
	PaybackMonth   *int              `json:"paybackMonth"`
	MonthlyIRR     *float64          `json:"monthlyIrr"`
	Viability      finance.Viability `json:"viability"`
}

var sensitivityDeltas = []float64{-0.2, -0.1, 0, 0.1, 0.2}

func blocked(sv store.SettingsVersion, err error, extra ...string) Analysis {
	blockers := extra
	if err != nil {
		blockers = append(blockers, strings.Split(err.Error(), "\n")...)
	}
	return Analysis{Status: "blocked", Blockers: blockers, SettingsVersionID: sv.ID}
}

// analyze roda o motor financeiro para um projeto.
func analyze(p store.Project, items []store.Item, sv store.SettingsVersion) Analysis {
	rate := 0.0
	if p.ExchangeRate != nil {
		rate = *p.ExchangeRate
	}
	var capexItems []finance.Item
	var licenses []finance.PointsLicense
	for _, it := range items {
		if it.Kind == string(finance.KindPointsLicense) {
			if it.License == nil {
				return blocked(sv, nil, it.Name+": licença por pontos sem premissas")
			}
			daily := it.License.DailyPoints
			licenses = append(licenses, finance.PointsLicense{
				Name: it.Name, DailyPoints: &daily, PriceUSD: it.License.PriceUSD,
				ExchangeRate: rate, Quantity: it.Quantity,
			})
			continue
		}
		origin := ""
		if it.OriginState != nil {
			origin = *it.OriginState
		}
		capexItems = append(capexItems, finance.Item{
			Name: it.Name, Kind: finance.ItemKind(it.Kind), Quantity: it.Quantity, UnitPrice: it.UnitPrice,
			Currency: finance.Currency(it.Currency), OriginState: origin, Imported: it.Imported,
		})
	}

	var pre []string
	if len(licenses) > 0 {
		if p.ExchangeRate == nil {
			pre = append(pre, "Informe o câmbio USD/BRL da proposta: há licenças por pontos.")
		}
		if p.ContractStartDate == nil {
			pre = append(pre, "Informe a data de início do contrato: há licenças por pontos.")
		}
	}
	if len(pre) > 0 {
		return blocked(sv, nil, pre...)
	}

	capex, err := finance.ComputeCapex(capexItems, finance.CapexParams{
		ExchangeRate: rate, DestState: p.DestState, InternalICMS: sv.Policy.InternalICMS,
	})
	if err != nil {
		return blocked(sv, err)
	}
	start := ""
	if p.ContractStartDate != nil {
		start = *p.ContractStartDate
	}
	licensing, err := finance.EstimateProjectLicensing(licenses, p.DurationMonths, start)
	if err != nil {
		return blocked(sv, err)
	}
	input := finance.Input{
		Capex:                 capex.Total,
		MonthlyRevenue:        p.MonthlyRevenue,
		DurationMonths:        p.DurationMonths,
		ImplementationRevenue: p.ImplementationRevenue,
		LossRate:              p.ExpectedLossRate,
		SupportCost:           p.MonthlySupportCost,
		LicensingCosts:        licensing.MonthlyCosts,
		Settings:              sv.Policy.Settings,
	}
	scenarios, err := finance.CalculateScenarios(input)
	if err != nil {
		return blocked(sv, err)
	}
	var sensitivity []SensitivityRow
	for _, delta := range sensitivityDeltas {
		varied := input
		varied.MonthlyRevenue = math.Round(input.MonthlyRevenue*(1+delta)*100) / 100
		r, err := finance.Calculate(varied)
		if err != nil {
			return blocked(sv, err)
		}
		sensitivity = append(sensitivity, SensitivityRow{
			Delta: delta, MonthlyRevenue: varied.MonthlyRevenue, TotalRevenue: r.TotalRevenue, NPV: r.NPV,
			PaybackMonth: r.PaybackMonth, MonthlyIRR: r.MonthlyIRR, Viability: r.Viability,
		})
	}
	// A decisão do portal é sempre a do cenário pessimista.
	health := finance.ContractHealth(p.DurationMonths, scenarios.Pessimistic.PaybackMonth)
	irr := finance.AssessIRR(scenarios.Pessimistic.MonthlyIRR, sv.Policy.Settings.IRRAlertThreshold)
	return Analysis{
		Status: "ready", SettingsVersionID: sv.ID, Capex: &capex, Licensing: &licensing,
		Scenarios: &scenarios, Health: &health, IRR: &irr, Sensitivity: sensitivity,
	}
}

// Summary é o resumo de um projeto para listas e para a carteira.
type Summary struct {
	Status       string               `json:"status"`
	Capex        float64              `json:"capex"`
	TotalRevenue float64              `json:"totalRevenue"`
	NPV          float64              `json:"npv"`
	PaybackMonth *int                 `json:"paybackMonth"`
	MonthlyIRR   *float64             `json:"monthlyIrr"`
	Viability    finance.Viability    `json:"viability,omitempty"`
	Health       finance.HealthStatus `json:"health,omitempty"`
	IRRStatus    finance.IRRStatus    `json:"irrStatus,omitempty"`
}

func summarize(a Analysis) Summary {
	if a.Status != "ready" {
		return Summary{Status: a.Status}
	}
	p := a.Scenarios.Pessimistic
	return Summary{
		Status: a.Status, Capex: p.Capex, TotalRevenue: p.TotalRevenue, NPV: p.NPV,
		PaybackMonth: p.PaybackMonth, MonthlyIRR: p.MonthlyIRR, Viability: p.Viability,
		Health: a.Health.Status, IRRStatus: a.IRR.Status,
	}
}

// settingsFor devolve a política de um projeto: a congelada, se houver, ou a
// vigente.
func settingsFor(p store.Project, current store.SettingsVersion, pinned map[int64]store.SettingsVersion) store.SettingsVersion {
	if p.SettingsVersionID != nil {
		if v, ok := pinned[*p.SettingsVersionID]; ok {
			return v
		}
	}
	return current
}

type projectRow struct {
	Project store.Project `json:"project"`
	Access  string        `json:"access"`
	Summary Summary       `json:"summary"`
}

// visibleRows carrega os projetos que o usuário pode ler, já analisados.
func (s *Server) visibleRows(r *http.Request, u store.User) ([]projectRow, store.SettingsVersion, error) {
	ctx := r.Context()
	current, err := s.store.CurrentSettings(ctx)
	if err != nil {
		return nil, current, err
	}
	projects, err := s.store.ProjectsVisibleTo(ctx, u.ID, u.IsAdmin())
	if err != nil {
		return nil, current, err
	}
	ids := make([]int64, len(projects))
	var pinnedIDs []int64
	for i, p := range projects {
		ids[i] = p.ID
		if p.SettingsVersionID != nil {
			pinnedIDs = append(pinnedIDs, *p.SettingsVersionID)
		}
	}
	items, err := s.store.ItemsByProjects(ctx, ids)
	if err != nil {
		return nil, current, err
	}
	pinned, err := s.store.SettingsByIDs(ctx, pinnedIDs)
	if err != nil {
		return nil, current, err
	}
	rows := make([]projectRow, len(projects))
	for i, p := range projects {
		access := accessShared
		switch {
		case p.OwnerID == u.ID:
			access = accessOwner
		case u.IsAdmin():
			access = accessAdmin
		}
		rows[i] = projectRow{Project: p, Access: access, Summary: summarize(analyze(p, items[p.ID], settingsFor(p, current, pinned)))}
	}
	return rows, current, nil
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, u store.User) error {
	rows, _, err := s.visibleRows(r, u)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, rows)
	return nil
}

// portfolioTotals são os indicadores agregados de um status da carteira.
type portfolioTotals struct {
	Count        int     `json:"count"`
	Blocked      int     `json:"blocked"`
	Viable       int     `json:"viable"`
	Exception    int     `json:"exception"`
	NotViable    int     `json:"notViable"`
	BelowIRR     int     `json:"belowIrrThreshold"`
	Capex        float64 `json:"capex"`
	TotalRevenue float64 `json:"totalRevenue"`
	NPV          float64 `json:"npv"`
}

// dashboard agrega a carteira por status operacional. Projetos de teste ficam
// de fora e são só contados, para que rascunhos e testes não inflem os
// indicadores de contratos reais.
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, u store.User) error {
	rows, current, err := s.visibleRows(r, u)
	if err != nil {
		return err
	}
	byStatus := map[string]*portfolioTotals{
		store.StatusDraft: {}, store.StatusActive: {}, store.StatusCompleted: {}, store.StatusCancelled: {},
	}
	tests := 0
	for _, row := range rows {
		if row.Project.IsTest {
			tests++
			continue
		}
		t := byStatus[row.Project.Status]
		t.Count++
		sum := row.Summary
		if sum.Status != "ready" {
			t.Blocked++
			continue
		}
		switch sum.Viability {
		case finance.Viable:
			t.Viable++
		case finance.CommercialException:
			t.Exception++
		default:
			t.NotViable++
		}
		if sum.IRRStatus == finance.IRRBelowThreshold {
			t.BelowIRR++
		}
		t.Capex += sum.Capex
		t.TotalRevenue += sum.TotalRevenue
		t.NPV += sum.NPV
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"byStatus":          byStatus,
		"testProjects":      tests,
		"opportunityRate":   current.Policy.Settings.OpportunityRate,
		"irrAlertThreshold": current.Policy.Settings.IRRAlertThreshold,
	})
	return nil
}

// difal é a calculadora avulsa de DIFAL.
func (s *Server) difal(w http.ResponseWriter, r *http.Request, _ store.User) error {
	q := r.URL.Query()
	base, err := strconv.ParseFloat(q.Get("base"), 64)
	if err != nil {
		return badRequest("Informe o valor base.")
	}
	dest := q.Get("dest")
	if dest == "" {
		dest = "SP"
	}
	current, err := s.store.CurrentSettings(r.Context())
	if err != nil {
		return err
	}
	res, err := finance.Difal(base, q.Get("origin"), dest, q.Get("imported") == "true", current.Policy.InternalICMS)
	if err != nil {
		return badRequest(err.Error())
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// simulate recalcula o projeto com outra mensalidade, sem gravar nada. É o
// simulador "e se eu cobrar X?" da tela do projeto.
func (s *Server) simulate(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, _, err := s.projectFor(r, u)
	if err != nil {
		return err
	}
	monthly, err := strconv.ParseFloat(r.URL.Query().Get("monthlyRevenue"), 64)
	if err != nil || !validAmount(monthly) {
		return badRequest("Informe a mensalidade a simular.")
	}
	items, err := s.store.Items(r.Context(), p.ID)
	if err != nil {
		return err
	}
	sv, err := s.projectSettings(r, p)
	if err != nil {
		return err
	}
	p.MonthlyRevenue = monthly
	writeJSON(w, http.StatusOK, analyze(p, items, sv))
	return nil
}
