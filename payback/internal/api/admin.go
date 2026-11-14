package api

import (
	"math"
	"net/http"
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
	saved, err := s.store.SaveSettings(r.Context(), in.BaseID, in.Policy, u.ID)
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
	p, err := s.store.UpdateProgramPrice(r.Context(), r.PathValue("key"), in.PriceUSD, in.Revision)
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
	m, err := s.store.UpdateModelConsumption(r.Context(), id, in.DailyPoints, service, in.Revision)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}
