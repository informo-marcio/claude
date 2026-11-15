package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/informo-marcio/claude/payback/internal/store"
)

const (
	maxAmount   = 1e12
	maxTextLen  = 200
	maxNotesLen = 5000
)

// projectInput é o corpo de criação e edição de projeto.
type projectInput struct {
	ClientName            string  `json:"clientName"`
	Name                  string  `json:"name"`
	Status                string  `json:"status"`
	IsTest                bool    `json:"isTest"`
	DurationMonths        int     `json:"durationMonths"`
	MonthlyRevenue        float64 `json:"monthlyRevenue"`
	ImplementationRevenue float64 `json:"implementationRevenue"`
	// Nulos na criação recebem o padrão da política financeira.
	ExpectedLossRate   *float64 `json:"expectedLossRate"`
	MonthlySupportCost *float64 `json:"monthlySupportCost"`
	ExchangeRate       *float64 `json:"exchangeRate"`
	DestState          string   `json:"destState"`
	ContractStartDate  *string  `json:"contractStartDate"`
	DealID             *int64   `json:"dealId"`
	Notes              string   `json:"notes"`
	// Version é a versão lida pelo usuário; obrigatória na edição.
	Version int `json:"version"`
}

func validAmount(v float64) bool { return v >= 0 && v <= maxAmount }

// toProject valida o corpo e o converte em projeto.
func (in projectInput) toProject(sv store.SettingsVersion) (store.Project, error) {
	var errs []string
	p := store.Project{
		ClientName: strings.TrimSpace(in.ClientName), Name: strings.TrimSpace(in.Name),
		Status: in.Status, IsTest: in.IsTest, DurationMonths: in.DurationMonths,
		MonthlyRevenue: in.MonthlyRevenue, ImplementationRevenue: in.ImplementationRevenue,
		ExchangeRate: in.ExchangeRate, DestState: strings.ToUpper(strings.TrimSpace(in.DestState)),
		ContractStartDate: in.ContractStartDate, DealID: in.DealID, Notes: strings.TrimSpace(in.Notes),
		Version: in.Version,
	}
	policy := sv.Policy.Settings
	p.ExpectedLossRate, p.MonthlySupportCost = policy.DefaultLossRate, policy.DefaultSupportCost
	if in.ExpectedLossRate != nil {
		p.ExpectedLossRate = *in.ExpectedLossRate
	}
	if in.MonthlySupportCost != nil {
		p.MonthlySupportCost = *in.MonthlySupportCost
	}
	if p.Status == "" {
		p.Status = store.StatusDraft
	}
	if p.DestState == "" {
		p.DestState = "SP"
	}

	if p.ClientName == "" || len(p.ClientName) > maxTextLen {
		errs = append(errs, "Informe o cliente (até 200 caracteres).")
	}
	if p.Name == "" || len(p.Name) > maxTextLen {
		errs = append(errs, "Informe o nome do projeto (até 200 caracteres).")
	}
	if len(p.Notes) > maxNotesLen {
		errs = append(errs, "As observações passam de 5.000 caracteres.")
	}
	switch p.Status {
	case store.StatusDraft, store.StatusActive, store.StatusCompleted, store.StatusCancelled:
	default:
		errs = append(errs, "Status inválido.")
	}
	durationOK := false
	for _, d := range policy.Durations {
		durationOK = durationOK || d == p.DurationMonths
	}
	if !durationOK {
		errs = append(errs, fmt.Sprintf("Prazo de %d meses não está na política financeira.", p.DurationMonths))
	}
	if !validAmount(p.MonthlyRevenue) {
		errs = append(errs, "Mensalidade inválida.")
	}
	if !validAmount(p.ImplementationRevenue) {
		errs = append(errs, "Receita de implantação inválida.")
	}
	if !validAmount(p.MonthlySupportCost) {
		errs = append(errs, "Custo de suporte inválido.")
	}
	if p.ExpectedLossRate < 0 || p.ExpectedLossRate > 1 {
		errs = append(errs, "Inadimplência esperada deve estar entre 0 e 100%.")
	}
	if p.ExchangeRate != nil && (*p.ExchangeRate <= 0 || *p.ExchangeRate > 1000) {
		errs = append(errs, "Câmbio USD/BRL inválido.")
	}
	if _, ok := sv.Policy.InternalICMS[p.DestState]; !ok {
		errs = append(errs, "UF de destino inválida.")
	}
	if p.ContractStartDate != nil {
		if d, err := time.Parse("2006-01-02", *p.ContractStartDate); err != nil || d.Year() < 2000 || d.Year() > 2100 {
			errs = append(errs, "Data de início inválida (use AAAA-MM-DD).")
		}
	}
	if p.DealID != nil && *p.DealID < 1 {
		errs = append(errs, "Deal ID inválido.")
	}
	if len(errs) > 0 {
		return p, badRequest("Revise os dados do projeto.", errs...)
	}
	return p, nil
}

type projectDetail struct {
	Project  store.Project `json:"project"`
	Access   string        `json:"access"`
	Items    []store.Item  `json:"items"`
	Analysis Analysis      `json:"analysis"`
}

// detail monta o projeto com itens e análise.
func (s *Server) detail(r *http.Request, p store.Project, access string) (projectDetail, error) {
	ctx := r.Context()
	items, err := s.store.Items(ctx, p.ID)
	if err != nil {
		return projectDetail{}, err
	}
	sv, err := s.projectSettings(r, p)
	if err != nil {
		return projectDetail{}, err
	}
	return projectDetail{Project: p, Access: access, Items: items, Analysis: analyze(p, items, sv)}, nil
}

// projectSettings devolve a política congelada do projeto ou a vigente.
func (s *Server) projectSettings(r *http.Request, p store.Project) (store.SettingsVersion, error) {
	ctx := r.Context()
	if p.SettingsVersionID != nil {
		m, err := s.store.SettingsByIDs(ctx, []int64{*p.SettingsVersionID})
		if err != nil {
			return store.SettingsVersion{}, err
		}
		if v, ok := m[*p.SettingsVersionID]; ok {
			return v, nil
		}
	}
	return s.store.CurrentSettings(ctx)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, access, err := s.projectFor(r, u)
	if err != nil {
		return err
	}
	d, err := s.detail(r, p, access)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, u store.User) error {
	var in projectInput
	if err := readJSON(r, &in); err != nil {
		return err
	}
	current, err := s.store.CurrentSettings(r.Context())
	if err != nil {
		return err
	}
	// Todo projeto nasce como rascunho; ativar exige análise pronta.
	in.Status = store.StatusDraft
	p, err := in.toProject(current)
	if err != nil {
		return err
	}
	created, err := s.store.CreateProject(r.Context(), p, actorFor(u, false))
	if err != nil {
		return err
	}
	d, err := s.detail(r, created, accessOwner)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, d)
	return nil
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request, u store.User) error {
	old, access, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	var in projectInput
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.ExpectedLossRate == nil {
		in.ExpectedLossRate = &old.ExpectedLossRate
	}
	if in.MonthlySupportCost == nil {
		in.MonthlySupportCost = &old.MonthlySupportCost
	}
	current, err := s.store.CurrentSettings(r.Context())
	if err != nil {
		return err
	}
	// Um contrato fechado continua na política em que foi ativado.
	sv, err := s.projectSettings(r, old)
	if err != nil {
		return err
	}
	if in.Status == store.StatusDraft || in.Status == "" {
		sv = current
	}
	p, err := in.toProject(sv)
	if err != nil {
		return err
	}
	p.ID = old.ID

	switch {
	case p.Status == store.StatusDraft:
		p.SettingsVersionID = nil
	case old.SettingsVersionID != nil:
		p.SettingsVersionID = old.SettingsVersionID
	default:
		// Saindo de rascunho: congela a política vigente, e só com análise pronta.
		p.SettingsVersionID = &current.ID
		items, err := s.store.Items(r.Context(), p.ID)
		if err != nil {
			return err
		}
		if a := analyze(p, items, current); a.Status != "ready" {
			return badRequest("O projeto só pode sair de rascunho com a análise completa.", a.Blockers...)
		}
	}

	updated, err := s.store.UpdateProject(r.Context(), p, actor, describeChanges(old, p))
	if err != nil {
		return err
	}
	d, err := s.detail(r, updated, access)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

var statusLabel = map[string]string{
	store.StatusDraft: "rascunho", store.StatusActive: "ativo",
	store.StatusCompleted: "concluído", store.StatusCancelled: "cancelado",
}

// describeChanges resume, para o histórico, o que mudou no projeto.
func describeChanges(old, p store.Project) string {
	var parts []string
	if old.ClientName != p.ClientName || old.Name != p.Name {
		parts = append(parts, fmt.Sprintf("identificação: %s — %s → %s — %s", old.ClientName, old.Name, p.ClientName, p.Name))
	}
	if old.IsTest != p.IsTest {
		parts = append(parts, fmt.Sprintf("marcado como teste: %v → %v", old.IsTest, p.IsTest))
	}
	if old.Status != p.Status {
		parts = append(parts, fmt.Sprintf("status: %s → %s", statusLabel[old.Status], statusLabel[p.Status]))
	}
	if old.MonthlyRevenue != p.MonthlyRevenue {
		parts = append(parts, fmt.Sprintf("mensalidade: %s → %s", money(old.MonthlyRevenue), money(p.MonthlyRevenue)))
	}
	if old.DurationMonths != p.DurationMonths {
		parts = append(parts, fmt.Sprintf("prazo: %d → %d meses", old.DurationMonths, p.DurationMonths))
	}
	if old.ImplementationRevenue != p.ImplementationRevenue {
		parts = append(parts, fmt.Sprintf("implantação: %s → %s", money(old.ImplementationRevenue), money(p.ImplementationRevenue)))
	}
	if !equalPtr(old.ExchangeRate, p.ExchangeRate) {
		parts = append(parts, fmt.Sprintf("câmbio: %s → %s", fmtRate(old.ExchangeRate), fmtRate(p.ExchangeRate)))
	}
	if old.ExpectedLossRate != p.ExpectedLossRate {
		parts = append(parts, fmt.Sprintf("inadimplência: %.2f%% → %.2f%%", old.ExpectedLossRate*100, p.ExpectedLossRate*100))
	}
	if old.MonthlySupportCost != p.MonthlySupportCost {
		parts = append(parts, fmt.Sprintf("suporte: %s → %s", money(old.MonthlySupportCost), money(p.MonthlySupportCost)))
	}
	if old.DestState != p.DestState {
		parts = append(parts, fmt.Sprintf("UF de destino: %s → %s", old.DestState, p.DestState))
	}
	if !equalStr(old.ContractStartDate, p.ContractStartDate) {
		parts = append(parts, "data de início alterada")
	}
	if len(parts) == 0 {
		return "Dados do projeto atualizados"
	}
	return "Projeto atualizado: " + strings.Join(parts, "; ")
}

func fmtRate(v *float64) string {
	if v == nil {
		return "sem câmbio"
	}
	return fmt.Sprintf("%.4f", *v)
}

func equalStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, _, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProject(r.Context(), p.ID, actor); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, _, err := s.projectFor(r, u)
	if err != nil {
		return err
	}
	events, err := s.store.Events(r.Context(), p.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, events)
	return nil
}

func (s *Server) listShares(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, _, _, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	shares, err := s.store.Shares(r.Context(), p.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, shares)
	return nil
}

// addShare compartilha por e-mail exato. Não há busca de usuários, para que o
// portal não sirva de lista de e-mails da empresa.
func (s *Server) addShare(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, _, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	target, err := s.store.UserByEmail(r.Context(), in.Email)
	if err != nil {
		if err == store.ErrNotFound {
			return badRequest("Essa pessoa ainda não acessou o portal. Peça que ela entre uma vez e tente de novo.")
		}
		return err
	}
	if target.ID == p.OwnerID {
		return badRequest("Essa pessoa já é a dona do projeto.")
	}
	if err := s.store.AddShare(r.Context(), p.ID, target, actor); err != nil {
		return err
	}
	shares, err := s.store.Shares(r.Context(), p.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, shares)
	return nil
}

func (s *Server) removeShare(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, _, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	userID, err := pathID(r, "userId")
	if err != nil {
		return err
	}
	if err := s.store.RemoveShare(r.Context(), p.ID, userID, actor); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
