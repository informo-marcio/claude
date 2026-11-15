package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/informo-marcio/claude/payback/internal/finance"
	"github.com/informo-marcio/claude/payback/internal/store"
)

// itemInput é o corpo de criação e edição de item.
type itemInput struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"`
	Currency    string  `json:"currency"`
	OriginState string  `json:"originState"`
	Imported    bool    `json:"imported"`
	// ModelID é o modelo do catálogo, para licença por pontos.
	ModelID int64 `json:"modelId"`
	// RefreshLicense recopia preço e consumo do catálogo em uma edição.
	RefreshLicense bool `json:"refreshLicense"`
}

// toItem valida o corpo. existing é o item atual em uma edição.
func (s *Server) toItem(r *http.Request, in itemInput, projectID int64, existing *store.Item) (store.Item, error) {
	it := store.Item{
		ProjectID: projectID, Name: strings.TrimSpace(in.Name), Kind: in.Kind, Quantity: in.Quantity,
		UnitPrice: in.UnitPrice, Currency: in.Currency, Imported: in.Imported,
	}
	var errs []string
	if it.Name == "" || len(it.Name) > maxTextLen {
		errs = append(errs, "Informe o nome do item (até 200 caracteres).")
	}
	if it.Quantity < 1 || it.Quantity > 10000 {
		errs = append(errs, "Quantidade deve estar entre 1 e 10.000.")
	}

	switch finance.ItemKind(it.Kind) {
	case finance.KindHardware, finance.KindSoftware:
		if it.Currency == "" {
			it.Currency = string(finance.BRL)
		}
		if it.Currency != string(finance.BRL) && it.Currency != string(finance.USD) {
			errs = append(errs, "Moeda deve ser BRL ou USD.")
		}
		if !validAmount(it.UnitPrice) {
			errs = append(errs, "Preço unitário inválido.")
		}
		if it.Kind == string(finance.KindHardware) {
			current, err := s.store.CurrentSettings(r.Context())
			if err != nil {
				return it, err
			}
			uf := strings.ToUpper(strings.TrimSpace(in.OriginState))
			if _, ok := current.Policy.InternalICMS[uf]; !ok {
				errs = append(errs, "Informe a UF de origem do equipamento.")
			}
			it.OriginState = &uf
		} else {
			it.Imported = false
		}
	case finance.KindPointsLicense:
		it.UnitPrice, it.Currency, it.Imported = 0, string(finance.BRL), false
		keep := existing != nil && existing.License != nil && existing.License.ModelID == in.ModelID && !in.RefreshLicense
		if keep {
			// Editar quantidade ou nome não reprecifica a proposta.
			it.License = existing.License
			break
		}
		model, program, err := s.store.ModelWithProgram(r.Context(), in.ModelID)
		if err != nil {
			if err == store.ErrNotFound {
				errs = append(errs, "Escolha um modelo do catálogo.")
				break
			}
			return it, err
		}
		if model.DailyPoints == nil {
			errs = append(errs, fmt.Sprintf("%s ainda não tem consumo de pontos cadastrado no catálogo.", model.Model))
			break
		}
		it.License = &store.License{
			ProgramKey: program.Key, ModelID: model.ID, ModelName: model.Model,
			DailyPoints: *model.DailyPoints, PriceUSD: program.PriceUSD, ServiceDescription: model.ServiceDescription,
		}
	default:
		errs = append(errs, "Tipo de item inválido.")
	}
	if len(errs) > 0 {
		return it, badRequest("Revise os dados do item.", errs...)
	}
	return it, nil
}

var kindLabel = map[string]string{
	"hardware": "hardware", "software": "software", "points_license": "licença por pontos",
}

func describeItem(verb string, it store.Item) string {
	return fmt.Sprintf("Item %s: %s (%s) x%d", verb, it.Name, kindLabel[it.Kind], it.Quantity)
}

// respondDetail devolve o projeto atualizado, para a tela não precisar de uma
// segunda chamada depois de alterar itens.
func (s *Server) respondDetail(w http.ResponseWriter, r *http.Request, projectID int64, access string, status int) error {
	p, err := s.store.Project(r.Context(), projectID)
	if err != nil {
		return err
	}
	d, err := s.detail(r, p, access)
	if err != nil {
		return err
	}
	writeJSON(w, status, d)
	return nil
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, access, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	var in itemInput
	if err := readJSON(r, &in); err != nil {
		return err
	}
	it, err := s.toItem(r, in, p.ID, nil)
	if err != nil {
		return err
	}
	if _, err := s.store.CreateItem(r.Context(), it, actor, describeItem("adicionado", it)); err != nil {
		return err
	}
	return s.respondDetail(w, r, p.ID, access, http.StatusCreated)
}

func (s *Server) updateItem(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, access, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	itemID, err := pathID(r, "itemId")
	if err != nil {
		return err
	}
	existing, err := s.store.Item(r.Context(), p.ID, itemID)
	if err != nil {
		return err
	}
	var in itemInput
	if err := readJSON(r, &in); err != nil {
		return err
	}
	it, err := s.toItem(r, in, p.ID, &existing)
	if err != nil {
		return err
	}
	it.ID = itemID
	if _, err := s.store.UpdateItem(r.Context(), it, actor, describeItem("alterado", it)); err != nil {
		return err
	}
	return s.respondDetail(w, r, p.ID, access, http.StatusOK)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, access, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	itemID, err := pathID(r, "itemId")
	if err != nil {
		return err
	}
	existing, err := s.store.Item(r.Context(), p.ID, itemID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteItem(r.Context(), p.ID, itemID, actor, describeItem("removido", existing)); err != nil {
		return err
	}
	return s.respondDetail(w, r, p.ID, access, http.StatusOK)
}

func (s *Server) duplicateItem(w http.ResponseWriter, r *http.Request, u store.User) error {
	p, access, actor, err := s.editableProject(r, u)
	if err != nil {
		return err
	}
	itemID, err := pathID(r, "itemId")
	if err != nil {
		return err
	}
	if _, err := s.store.DuplicateItem(r.Context(), p.ID, itemID, actor); err != nil {
		return err
	}
	return s.respondDetail(w, r, p.ID, access, http.StatusCreated)
}
