package api

import (
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/informo-marcio/claude/payback/internal/store"
)

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, _ store.User) error {
	current, err := s.store.CurrentSettings(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, current)
	return nil
}

// putSettings grava uma nova versão da política. Rascunhos passam a usá-la na
// próxima leitura; contratos já ativados continuam na versão congelada.
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, u store.User) error {
	if err := requireAdmin(u); err != nil {
		return err
	}
	var in struct {
		// BaseID é a versão que o administrador estava editando.
		BaseID int64        `json:"baseId"`
		Policy store.Policy `json:"policy"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := in.Policy.Validate(); err != nil {
		return badRequest("Revise a política financeira.", strings.Split(err.Error(), "\n")...)
	}
	previous, err := s.store.CurrentSettings(r.Context())
	if err != nil {
		return err
	}
	saved, err := s.store.SaveSettings(r.Context(), in.BaseID, in.Policy, actorFor(u, false), describePolicyChange(previous.Policy, in.Policy))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, saved)
	return nil
}

func (s *Server) getCatalog(w http.ResponseWriter, r *http.Request, _ store.User) error {
	c, err := s.store.Catalog(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, c)
	return nil
}

// validCatalogNumber aceita valores positivos com até seis casas decimais.
func validCatalogNumber(v float64) bool {
	return v > 0 && v <= 999999 && math.Abs(v*1e6-math.Round(v*1e6)) < 1e-4
}

func (s *Server) putProgram(w http.ResponseWriter, r *http.Request, u store.User) error {
	if err := requireAdmin(u); err != nil {
		return err
	}
	var in struct {
		PriceUSD float64 `json:"priceUsd"`
		Revision int     `json:"revision"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if !validCatalogNumber(in.PriceUSD) {
		return badRequest("Preço do ponto inválido (use até seis casas decimais).")
	}
	p, err := s.store.UpdateProgramPrice(r.Context(), r.PathValue("key"), in.PriceUSD, in.Revision, actorFor(u, false))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (s *Server) putModel(w http.ResponseWriter, r *http.Request, u store.User) error {
	if err := requireAdmin(u); err != nil {
		return err
	}
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		DailyPoints        *float64 `json:"dailyPoints"`
		ServiceDescription string   `json:"serviceDescription"`
		Revision           int      `json:"revision"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	service := strings.TrimSpace(in.ServiceDescription)
	if len(service) > 255 {
		return badRequest("A descrição do pacote passa de 255 caracteres.")
	}
	if in.DailyPoints != nil {
		if !validCatalogNumber(*in.DailyPoints) {
			return badRequest("Consumo diário inválido (use até seis casas decimais).")
		}
		if service == "" {
			return badRequest("Informe o pacote de serviços correspondente ao consumo diário.")
		}
	}
	m, err := s.store.UpdateModelConsumption(r.Context(), id, in.DailyPoints, service, in.Revision, actorFor(u, false))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}

// describePolicyChange resume, para a auditoria, o que mudou na política.
func describePolicyChange(old, p store.Policy) string {
	var parts []string
	pct := func(label string, a, b float64) {
		if a != b {
			parts = append(parts, fmt.Sprintf("%s: %.2f%% → %.2f%%", label, a*100, b*100))
		}
	}
	o, n := old.Settings, p.Settings
	pct("impostos", o.TaxRate, n.TaxRate)
	pct("taxa de oportunidade", o.OpportunityRate, n.OpportunityRate)
	pct("alerta de TIR", o.IRRAlertThreshold, n.IRRAlertThreshold)
	pct("inadimplência padrão", o.DefaultLossRate, n.DefaultLossRate)
	if o.DefaultSupportCost != n.DefaultSupportCost {
		parts = append(parts, fmt.Sprintf("suporte padrão: %s → %s", money(o.DefaultSupportCost), money(n.DefaultSupportCost)))
	}
	if !reflect.DeepEqual(o.SortedDurations(), n.SortedDurations()) {
		parts = append(parts, fmt.Sprintf("prazos: %v → %v", o.SortedDurations(), n.SortedDurations()))
	}
	if !reflect.DeepEqual(o.MaxPayback, n.MaxPayback) {
		parts = append(parts, "payback máximo por prazo alterado")
	}
	if !reflect.DeepEqual(o.Commission, n.Commission) {
		parts = append(parts, "comissões por prazo alteradas")
	}
	if !reflect.DeepEqual(old.InternalICMS, p.InternalICMS) {
		parts = append(parts, "alíquotas internas de ICMS alteradas")
	}
	if len(parts) == 0 {
		return "Política financeira regravada sem alterações"
	}
	return "Política financeira alterada: " + strings.Join(parts, "; ")
}

// auditLog devolve a trilha completa de alterações, só para administradores.
// Inclui projetos já excluídos e mudanças de política e catálogo.
func (s *Server) auditLog(w http.ResponseWriter, r *http.Request, u store.User) error {
	if err := requireAdmin(u); err != nil {
		return err
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if before < 0 {
		before = 0
	}
	events, err := s.store.AuditLog(r.Context(), before, 200)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, events)
	return nil
}
