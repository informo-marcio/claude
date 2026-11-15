package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/informo-marcio/claude/payback/internal/auth"
	"github.com/informo-marcio/claude/payback/internal/finance"
	"github.com/informo-marcio/claude/payback/internal/store"
)

// Estes testes rodam contra um PostgreSQL real, apontado por
// TEST_DATABASE_URL. Cada teste usa um schema próprio, criado e apagado aqui.

// headerAuth identifica o usuário pelo cabeçalho X-Test-User; só existe nos
// testes.
type headerAuth struct{}

func (headerAuth) Authenticate(r *http.Request) (auth.Identity, error) {
	email := r.Header.Get("X-Test-User")
	if email == "" {
		return auth.Identity{}, auth.ErrUnauthenticated
	}
	return auth.Identity{Email: email, Name: strings.Split(email, "@")[0]}, nil
}

const (
	owner = "dono@informo.test"
	other = "outro@informo.test"
	admin = "admin@informo.test"
)

type env struct {
	t      *testing.T
	srv    *httptest.Server
	st     *store.Store
	conn   *pgx.Conn
	schema string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("defina TEST_DATABASE_URL para rodar os testes de integração")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	st, err := store.Open(ctx, base+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrar duas vezes deve ser inofensivo: %v", err)
	}
	if err := st.SeedSettings(ctx); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(st, headerAuth{}, []string{admin}, log).Handler())
	t.Cleanup(func() {
		srv.Close()
		st.Close()
		conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") //nolint:errcheck
		conn.Close(ctx)
	})
	return &env{t: t, srv: srv, st: st, conn: conn, schema: schema}
}

// call faz uma requisição e decodifica a resposta em out (se não for nil).
func (e *env) call(user, method, path string, body, out any) int {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s: resposta inesperada (%d): %s", method, path, res.StatusCode, raw)
		}
	}
	return res.StatusCode
}

// must exige o status esperado.
func (e *env) must(want int, user, method, path string, body, out any) {
	e.t.Helper()
	var raw json.RawMessage
	got := e.call(user, method, path, body, &raw)
	if got != want {
		e.t.Fatalf("%s %s como %s: status %d, esperado %d: %s", method, path, user, got, want, raw)
	}
	if out != nil {
		// Zera o destino: campos omitidos na resposta não podem herdar valores
		// de uma chamada anterior.
		reflect.ValueOf(out).Elem().SetZero()
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatal(err)
		}
	}
}

func f(v float64) *float64 { return &v }

func baseProject() map[string]any {
	return map[string]any{
		"clientName": "ZOPONE", "name": "FORTIGATE 90G", "durationMonths": 36,
		"monthlyRevenue": 2500, "implementationRevenue": 3000,
		"expectedLossRate": 0, "monthlySupportCost": 0,
	}
}

func (e *env) createProject(user string, body map[string]any) projectDetail {
	e.t.Helper()
	var d projectDetail
	e.must(http.StatusCreated, user, "POST", "/api/projects", body, &d)
	return d
}

func (e *env) addItem(user string, projectID int64, body map[string]any) projectDetail {
	e.t.Helper()
	var d projectDetail
	e.must(http.StatusCreated, user, "POST", fmt.Sprintf("/api/projects/%d/items", projectID), body, &d)
	return d
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Errorf("%s = %.2f, esperado %.2f", name, got, want)
	}
}

// updateBody monta o corpo de edição a partir do projeto lido.
func updateBody(p store.Project) map[string]any {
	return map[string]any{
		"clientName": p.ClientName, "name": p.Name, "status": p.Status, "isTest": p.IsTest,
		"durationMonths": p.DurationMonths, "monthlyRevenue": p.MonthlyRevenue,
		"implementationRevenue": p.ImplementationRevenue, "expectedLossRate": p.ExpectedLossRate,
		"monthlySupportCost": p.MonthlySupportCost, "exchangeRate": p.ExchangeRate, "destState": p.DestState,
		"contractStartDate": p.ContractStartDate, "dealId": p.DealID, "notes": p.Notes, "version": p.Version,
	}
}

func TestProjectMatchesPreviousPortal(t *testing.T) {
	e := newEnv(t)
	d := e.createProject(owner, baseProject())
	if d.Project.Status != store.StatusDraft || d.Access != accessOwner {
		t.Fatalf("projeto criado: %+v", d.Project)
	}
	e.addItem(owner, d.Project.ID, map[string]any{"name": "Fortinet 90G", "kind": "hardware", "quantity": 1, "unitPrice": 9000, "originState": "SP"})
	d = e.addItem(owner, d.Project.ID, map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 25000})

	if d.Analysis.Status != "ready" {
		t.Fatalf("análise bloqueada: %v", d.Analysis.Blockers)
	}
	p := d.Analysis.Scenarios.Pessimistic
	near(t, "CAPEX", d.Analysis.Capex.Total, 34000)
	near(t, "VPL", p.NPV, 18534.27)
	near(t, "comissão", p.Commission.Total, 4050.14)
	if p.PaybackMonth == nil || *p.PaybackMonth != 23 || p.Viability != finance.Viable {
		t.Errorf("payback %v, viabilidade %s", p.PaybackMonth, p.Viability)
	}
	if d.Analysis.Health.Status != finance.HealthAdequate {
		t.Errorf("saúde = %s", d.Analysis.Health.Status)
	}

	var events []store.Event
	e.must(http.StatusOK, owner, "GET", fmt.Sprintf("/api/projects/%d/events", d.Project.ID), nil, &events)
	if len(events) != 3 {
		t.Errorf("histórico com %d eventos, esperado 3", len(events))
	}
}

func TestExchangeRateReconvertsItems(t *testing.T) {
	e := newEnv(t)
	body := baseProject()
	body["exchangeRate"] = 5
	d := e.createProject(owner, body)
	d = e.addItem(owner, d.Project.ID, map[string]any{
		"name": "Firewall", "kind": "hardware", "quantity": 2, "unitPrice": 500, "currency": "USD",
		"originState": "ES", "imported": true,
	})
	near(t, "hardware a 5,00", d.Analysis.Capex.Hardware, 5000)
	near(t, "DIFAL a 5,00 (importado, 14%)", d.Analysis.Capex.Difal, 700)

	upd := updateBody(d.Project)
	upd["exchangeRate"] = 6
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("/api/projects/%d", d.Project.ID), upd, &d)
	near(t, "hardware a 6,00", d.Analysis.Capex.Hardware, 6000)
	near(t, "DIFAL a 6,00", d.Analysis.Capex.Difal, 840)

	// Sem câmbio, um item em dólar bloqueia a análise em vez de virar zero.
	upd = updateBody(d.Project)
	upd["exchangeRate"] = nil
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("/api/projects/%d", d.Project.ID), upd, &d)
	if d.Analysis.Status != "blocked" || d.Analysis.Scenarios != nil {
		t.Errorf("análise deveria estar bloqueada sem câmbio: %+v", d.Analysis)
	}
}

func TestStaleVersionIsRejected(t *testing.T) {
	e := newEnv(t)
	d := e.createProject(owner, baseProject())
	path := fmt.Sprintf("/api/projects/%d", d.Project.ID)
	upd := updateBody(d.Project)
	upd["monthlyRevenue"] = 2600
	e.must(http.StatusOK, owner, "PUT", path, upd, nil)
	// Segunda gravação com a versão antiga: outra aba editou antes.
	upd["monthlyRevenue"] = 2700
	e.must(http.StatusConflict, owner, "PUT", path, upd, nil)
}

func TestProjectPermissions(t *testing.T) {
	e := newEnv(t)
	d := e.createProject(owner, baseProject())
	id := d.Project.ID
	path := fmt.Sprintf("/api/projects/%d", id)
	item := map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 100}

	e.must(http.StatusUnauthorized, "", "GET", path, nil, nil)

	// Quem não tem acesso não descobre nem que o projeto existe.
	e.must(http.StatusNotFound, other, "GET", path, nil, nil)
	e.must(http.StatusNotFound, other, "PUT", path, updateBody(d.Project), nil)
	e.must(http.StatusNotFound, other, "DELETE", path, map[string]any{}, nil)
	e.must(http.StatusNotFound, other, "POST", path+"/items", item, nil)
	var rows []projectRow
	e.must(http.StatusOK, other, "GET", "/api/projects", nil, &rows)
	if len(rows) != 0 {
		t.Errorf("outro usuário vê %d projetos", len(rows))
	}

	// Compartilhar concede leitura, nunca escrita.
	e.must(http.StatusOK, owner, "POST", path+"/shares", map[string]any{"email": other}, nil)
	var shared projectDetail
	e.must(http.StatusOK, other, "GET", path, nil, &shared)
	if shared.Access != accessShared {
		t.Errorf("acesso = %s", shared.Access)
	}
	e.must(http.StatusForbidden, other, "PUT", path, updateBody(d.Project), nil)
	e.must(http.StatusForbidden, other, "POST", path+"/items", item, nil)
	e.must(http.StatusForbidden, other, "DELETE", path, map[string]any{}, nil)
	e.must(http.StatusForbidden, other, "GET", path+"/shares", nil, nil)
	e.must(http.StatusForbidden, other, "POST", path+"/shares", map[string]any{"email": admin}, nil)

	// Retirar o compartilhamento retira o acesso.
	var shares []store.Share
	e.must(http.StatusOK, owner, "GET", path+"/shares", nil, &shares)
	if len(shares) != 1 {
		t.Fatalf("compartilhamentos = %d", len(shares))
	}
	e.must(http.StatusNoContent, owner, "DELETE", fmt.Sprintf("%s/shares/%d", path, shares[0].UserID), map[string]any{}, nil)
	e.must(http.StatusNotFound, other, "GET", path, nil, nil)

	e.must(http.StatusUnprocessableEntity, owner, "POST", path+"/shares", map[string]any{"email": "ninguem@informo.test"}, nil)
}

func TestAdminEditsAnyProjectWithAuditTrail(t *testing.T) {
	e := newEnv(t)
	d := e.createProject(owner, baseProject())
	id := d.Project.ID
	path := fmt.Sprintf("/api/projects/%d", id)

	// O administrador lê e edita projeto alheio, sem virar dono.
	var view projectDetail
	e.must(http.StatusOK, admin, "GET", path, nil, &view)
	if view.Access != accessAdmin {
		t.Fatalf("acesso do admin = %s", view.Access)
	}
	upd := updateBody(view.Project)
	upd["monthlyRevenue"] = 2800
	e.must(http.StatusOK, admin, "PUT", path, upd, &view)
	if view.Project.MonthlyRevenue != 2800 || view.Project.OwnerID != d.Project.OwnerID {
		t.Errorf("edição do admin: %+v", view.Project)
	}
	view = e.addItem(admin, id, map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 100})
	if len(view.Items) != 1 {
		t.Errorf("item incluído pelo admin: %+v", view.Items)
	}
	e.must(http.StatusUnprocessableEntity, admin, "POST", path+"/shares", map[string]any{"email": owner}, nil)

	// Quem só recebeu o compartilhamento continua sem editar.
	e.must(http.StatusOK, other, "GET", "/api/me", nil, nil) // primeiro acesso registra o usuário
	e.must(http.StatusOK, admin, "POST", path+"/shares", map[string]any{"email": other}, nil)
	e.must(http.StatusForbidden, other, "PUT", path, updateBody(view.Project), nil)

	// Usuário comum não ganha a edição de projeto alheio.
	e.must(http.StatusNotFound, "terceiro@informo.test", "PUT", path, updateBody(view.Project), nil)
}

func TestAuditTrail(t *testing.T) {
	e := newEnv(t)
	d := e.createProject(owner, baseProject())
	id := d.Project.ID
	path := fmt.Sprintf("/api/projects/%d", id)
	e.must(http.StatusOK, other, "GET", "/api/me", nil, nil) // registra o usuário

	var view projectDetail
	e.must(http.StatusOK, admin, "GET", path, nil, &view)
	upd := updateBody(view.Project)
	upd["monthlyRevenue"] = 2800
	e.must(http.StatusOK, admin, "PUT", path, upd, nil)
	e.addItem(admin, id, map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 100})
	e.must(http.StatusOK, admin, "POST", path+"/shares", map[string]any{"email": other}, nil)

	// O histórico do projeto mostra quem fez cada coisa e se foi como admin.
	var events []store.Event
	e.must(http.StatusOK, owner, "GET", path+"/events", nil, &events)
	if len(events) != 4 {
		t.Fatalf("histórico com %d eventos, esperado 4: %+v", len(events), events)
	}
	created, edited := events[3], events[2]
	if created.UserEmail != owner || created.AsAdmin {
		t.Errorf("criação: %+v", created)
	}
	if edited.UserEmail != admin || !edited.AsAdmin || !strings.Contains(edited.Description, "R$ 2.500,00 → R$ 2.800,00") {
		t.Errorf("edição do admin: %+v", edited)
	}
	if !strings.Contains(events[0].Description, other) {
		t.Errorf("compartilhamento: %+v", events[0])
	}

	// A trilha completa é só do administrador e sobrevive à exclusão.
	e.must(http.StatusForbidden, owner, "GET", "/api/audit", nil, nil)
	e.must(http.StatusNoContent, admin, "DELETE", path, map[string]any{}, nil)
	e.must(http.StatusNotFound, owner, "GET", path, nil, nil)
	var trail []store.Event
	e.must(http.StatusOK, admin, "GET", "/api/audit", nil, &trail)
	if len(trail) != 5 {
		t.Fatalf("trilha com %d registros, esperado 5", len(trail))
	}
	last := trail[0]
	if last.Description != "Projeto excluído" || !last.AsAdmin || last.UserEmail != admin ||
		last.ProjectLabel == nil || *last.ProjectLabel != "ZOPONE — FORTIGATE 90G" {
		t.Errorf("exclusão na trilha: %+v", last)
	}

	// Política e catálogo também deixam rastro.
	var current store.SettingsVersion
	e.must(http.StatusOK, admin, "GET", "/api/settings", nil, &current)
	current.Policy.Settings.TaxRate = 0.28
	e.must(http.StatusOK, admin, "PUT", "/api/settings", map[string]any{"baseId": current.ID, "policy": current.Policy}, nil)
	e.must(http.StatusOK, admin, "PUT", "/api/catalog/programs/fortinet", map[string]any{"priceUsd": 1.1, "revision": 0}, nil)
	e.must(http.StatusOK, admin, "GET", "/api/audit", nil, &trail)
	if !strings.Contains(trail[1].Description, "impostos: 26.50% → 28.00%") || !strings.Contains(trail[0].Description, "FortiFlex") {
		t.Errorf("política e catálogo na trilha: %q / %q", trail[1].Description, trail[0].Description)
	}

	// Nem a aplicação consegue reescrever ou apagar a trilha.
	ctx := context.Background()
	for _, sql := range []string{"UPDATE %s.audit_log SET description = 'x'", "DELETE FROM %s.audit_log", "TRUNCATE %s.audit_log"} {
		if _, err := e.conn.Exec(ctx, fmt.Sprintf(sql, e.schema)); err == nil {
			t.Errorf("a trilha aceitou: %s", sql)
		}
	}
}

func TestItemMustBelongToProject(t *testing.T) {
	e := newEnv(t)
	mine := e.createProject(owner, baseProject())
	theirs := e.createProject(other, baseProject())
	theirs = e.addItem(other, theirs.Project.ID, map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 100})
	foreign := theirs.Items[0].ID

	// O dono de um projeto não alcança itens de outro pelo id do item.
	base := fmt.Sprintf("/api/projects/%d/items/%d", mine.Project.ID, foreign)
	item := map[string]any{"name": "Invasão", "kind": "software", "quantity": 1, "unitPrice": 1}
	e.must(http.StatusNotFound, owner, "PUT", base, item, nil)
	e.must(http.StatusNotFound, owner, "DELETE", base, map[string]any{}, nil)
	e.must(http.StatusNotFound, owner, "POST", base+"/duplicate", map[string]any{}, nil)

	var check projectDetail
	e.must(http.StatusOK, other, "GET", fmt.Sprintf("/api/projects/%d", theirs.Project.ID), nil, &check)
	if len(check.Items) != 1 || check.Items[0].Name != "Licença" {
		t.Errorf("item alheio foi alterado: %+v", check.Items)
	}
}

func TestItemsDuplicateAndDelete(t *testing.T) {
	e := newEnv(t)
	d := e.createProject(owner, baseProject())
	id := d.Project.ID
	for _, name := range []string{"A", "B", "C"} {
		d = e.addItem(owner, id, map[string]any{"name": name, "kind": "software", "quantity": 1, "unitPrice": 100})
	}
	e.must(http.StatusCreated, owner, "POST", fmt.Sprintf("/api/projects/%d/items/%d/duplicate", id, d.Items[0].ID), map[string]any{}, &d)
	var names []string
	for _, it := range d.Items {
		names = append(names, it.Name)
	}
	if got := strings.Join(names, ","); got != "A,A (cópia),B,C" {
		t.Errorf("ordem após duplicar = %s", got)
	}
	e.must(http.StatusOK, owner, "DELETE", fmt.Sprintf("/api/projects/%d/items/%d", id, d.Items[1].ID), map[string]any{}, &d)
	if len(d.Items) != 3 {
		t.Errorf("itens após remover = %d", len(d.Items))
	}
	near(t, "CAPEX", d.Analysis.Capex.Total, 300)

	e.must(http.StatusNoContent, owner, "DELETE", fmt.Sprintf("/api/projects/%d", id), map[string]any{}, nil)
	e.must(http.StatusNotFound, owner, "GET", fmt.Sprintf("/api/projects/%d", id), nil, nil)
}

func TestPointsLicense(t *testing.T) {
	e := newEnv(t)
	var catalog store.Catalog
	e.must(http.StatusOK, owner, "GET", "/api/catalog", nil, &catalog)
	if len(catalog.Programs) != 2 || len(catalog.Models) != 20 {
		t.Fatalf("catálogo: %d programas, %d modelos", len(catalog.Programs), len(catalog.Models))
	}
	var ready, pending store.Model
	for _, m := range catalog.Models {
		if m.Model == "FortiGate 40F" {
			ready = m
		}
		if m.Model == "FortiGate 90G" {
			pending = m
		}
	}
	d := e.createProject(owner, baseProject())
	id := d.Project.ID
	items := fmt.Sprintf("/api/projects/%d/items", id)

	// Modelo sem consumo cadastrado não entra na proposta.
	e.must(http.StatusUnprocessableEntity, owner, "POST", items, map[string]any{"name": "90G", "kind": "points_license", "quantity": 1, "modelId": pending.ID}, nil)

	d = e.addItem(owner, id, map[string]any{"name": "FortiFlex 40F", "kind": "points_license", "quantity": 2, "modelId": ready.ID})
	if d.Analysis.Status != "blocked" || len(d.Analysis.Blockers) != 2 {
		t.Fatalf("sem câmbio e data a análise deveria ter 2 pendências: %+v", d.Analysis)
	}
	// Projeto com análise bloqueada não sai de rascunho.
	upd := updateBody(d.Project)
	upd["status"] = store.StatusActive
	e.must(http.StatusUnprocessableEntity, owner, "PUT", fmt.Sprintf("/api/projects/%d", id), upd, nil)

	upd = updateBody(d.Project)
	upd["exchangeRate"] = 5
	upd["contractStartDate"] = "2028-01-31"
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("/api/projects/%d", id), upd, &d)
	if d.Analysis.Status != "ready" {
		t.Fatalf("análise bloqueada: %v", d.Analysis.Blockers)
	}
	// 0,92 ponto/dia × 2 unidades × 29 dias × US$ 0,96 × 5,00.
	near(t, "licença no primeiro mês", d.Analysis.Licensing.MonthlyCosts[0], 0.92*2*29*0.96*5)
	near(t, "CAPEX sem licença por pontos", d.Analysis.Capex.Total, 0)
	before := d.Analysis.Licensing.TotalCostBRL

	// Alterar o catálogo exige administrador e não reprecifica a proposta.
	price := map[string]any{"priceUsd": 1.5, "revision": 0}
	e.must(http.StatusForbidden, owner, "PUT", "/api/catalog/programs/fortinet", price, nil)
	e.must(http.StatusOK, admin, "PUT", "/api/catalog/programs/fortinet", price, nil)
	e.must(http.StatusConflict, admin, "PUT", "/api/catalog/programs/fortinet", price, nil)
	e.must(http.StatusOK, owner, "GET", fmt.Sprintf("/api/projects/%d", id), nil, &d)
	near(t, "custo após mudar o catálogo", d.Analysis.Licensing.TotalCostBRL, before)

	// Editar a quantidade mantém o preço congelado; pedir atualização recopia.
	item := d.Items[0]
	edit := map[string]any{"name": item.Name, "kind": "points_license", "quantity": 4, "modelId": ready.ID}
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("%s/%d", items, item.ID), edit, &d)
	// O custo é arredondado mês a mês, então dobrar a quantidade pode diferir
	// do dobro exato por alguns centavos no contrato inteiro.
	if got := d.Analysis.Licensing.TotalCostBRL; math.Abs(got-before*2) > 0.36 {
		t.Errorf("custo com o dobro da quantidade = %.2f, esperado perto de %.2f", got, before*2)
	}
	edit["refreshLicense"] = true
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("%s/%d", items, item.ID), edit, &d)
	if d.Items[0].License.PriceUSD != 1.5 {
		t.Errorf("preço após atualizar = %v", d.Items[0].License.PriceUSD)
	}

	// Consumo exige descrição do pacote de serviços.
	model := fmt.Sprintf("/api/catalog/models/%d", pending.ID)
	e.must(http.StatusUnprocessableEntity, admin, "PUT", model, map[string]any{"dailyPoints": 1.2, "serviceDescription": "", "revision": 0}, nil)
	e.must(http.StatusOK, admin, "PUT", model, map[string]any{"dailyPoints": 1.2, "serviceDescription": "UTP", "revision": 0}, nil)
}

func TestSettingsVersioning(t *testing.T) {
	e := newEnv(t)
	mk := func() projectDetail {
		d := e.createProject(owner, baseProject())
		return e.addItem(owner, d.Project.ID, map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 34000})
	}
	draft, closed := mk(), mk()
	upd := updateBody(closed.Project)
	upd["status"] = store.StatusActive
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("/api/projects/%d", closed.Project.ID), upd, &closed)
	if closed.Project.SettingsVersionID == nil {
		t.Fatal("contrato ativado deveria congelar a política")
	}
	npvBefore := closed.Analysis.Scenarios.Pessimistic.NPV

	var current store.SettingsVersion
	e.must(http.StatusOK, owner, "GET", "/api/settings", nil, &current)
	change := map[string]any{"baseId": current.ID, "policy": current.Policy}
	e.must(http.StatusForbidden, owner, "PUT", "/api/settings", change, nil)

	current.Policy.Settings.TaxRate = 0.30
	change["policy"] = current.Policy
	e.must(http.StatusOK, admin, "PUT", "/api/settings", change, nil)
	// A mesma base de novo: outro administrador já gravou.
	e.must(http.StatusConflict, admin, "PUT", "/api/settings", change, nil)

	bad := current.Policy
	bad.Settings.TaxRate = 3
	e.must(http.StatusUnprocessableEntity, admin, "PUT", "/api/settings", map[string]any{"baseId": current.ID + 1, "policy": bad}, nil)

	e.must(http.StatusOK, owner, "GET", fmt.Sprintf("/api/projects/%d", closed.Project.ID), nil, &closed)
	near(t, "VPL do contrato ativo após mudar a política", closed.Analysis.Scenarios.Pessimistic.NPV, npvBefore)
	e.must(http.StatusOK, owner, "GET", fmt.Sprintf("/api/projects/%d", draft.Project.ID), nil, &draft)
	if got := draft.Analysis.Scenarios.Pessimistic.NPV; got >= npvBefore {
		t.Errorf("rascunho deveria usar a política nova: VPL %.2f", got)
	}

	// Voltar a rascunho solta a política congelada.
	upd = updateBody(closed.Project)
	upd["status"] = store.StatusDraft
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("/api/projects/%d", closed.Project.ID), upd, &closed)
	if closed.Project.SettingsVersionID != nil {
		t.Error("rascunho não deveria ter política congelada")
	}
}

func TestDashboardSeparatesStatusAndTests(t *testing.T) {
	e := newEnv(t)
	mk := func(test bool) projectDetail {
		body := baseProject()
		body["isTest"] = test
		d := e.createProject(owner, body)
		return e.addItem(owner, d.Project.ID, map[string]any{"name": "Licença", "kind": "software", "quantity": 1, "unitPrice": 34000})
	}
	mk(false)
	mk(true)
	active := mk(false)
	upd := updateBody(active.Project)
	upd["status"] = store.StatusActive
	e.must(http.StatusOK, owner, "PUT", fmt.Sprintf("/api/projects/%d", active.Project.ID), upd, nil)

	var dash struct {
		ByStatus     map[string]portfolioTotals `json:"byStatus"`
		TestProjects int                        `json:"testProjects"`
	}
	e.must(http.StatusOK, owner, "GET", "/api/dashboard", nil, &dash)
	if dash.TestProjects != 1 || dash.ByStatus["draft"].Count != 1 || dash.ByStatus["active"].Count != 1 {
		t.Fatalf("carteira: %+v", dash)
	}
	near(t, "CAPEX dos ativos", dash.ByStatus["active"].Capex, 34000)
	near(t, "VPL dos ativos", dash.ByStatus["active"].NPV, 18534.27)
	if dash.ByStatus["active"].Viable != 1 {
		t.Errorf("viáveis ativos = %d", dash.ByStatus["active"].Viable)
	}
}

func TestInputValidation(t *testing.T) {
	e := newEnv(t)
	bad := func(change func(map[string]any)) map[string]any {
		b := baseProject()
		change(b)
		return b
	}
	cases := map[string]map[string]any{
		"prazo fora da política": bad(func(b map[string]any) { b["durationMonths"] = 18 }),
		"mensalidade negativa":   bad(func(b map[string]any) { b["monthlyRevenue"] = -1 }),
		"sem cliente":            bad(func(b map[string]any) { b["clientName"] = "  " }),
		"perda acima de 100%":    bad(func(b map[string]any) { b["expectedLossRate"] = 1.5 }),
		"UF inválida":            bad(func(b map[string]any) { b["destState"] = "XX" }),
		"data inválida":          bad(func(b map[string]any) { b["contractStartDate"] = "31/01/2028" }),
	}
	for name, body := range cases {
		if got := e.call(owner, "POST", "/api/projects", body, nil); got != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, got)
		}
	}
	if got := e.call(owner, "POST", "/api/projects", bad(func(b map[string]any) { b["ownerId"] = 99 }), nil); got != http.StatusBadRequest {
		t.Errorf("campo desconhecido: status %d", got)
	}

	// Na criação, risco omitido recebe o padrão da política (0,3%).
	body := baseProject()
	delete(body, "expectedLossRate")
	d := e.createProject(owner, body)
	if d.Project.ExpectedLossRate != 0.003 {
		t.Errorf("inadimplência padrão = %v", d.Project.ExpectedLossRate)
	}

	id := d.Project.ID
	items := fmt.Sprintf("/api/projects/%d/items", id)
	for name, item := range map[string]map[string]any{
		"hardware sem UF":    {"name": "X", "kind": "hardware", "quantity": 1, "unitPrice": 10},
		"quantidade zero":    {"name": "X", "kind": "software", "quantity": 0, "unitPrice": 10},
		"tipo desconhecido":  {"name": "X", "kind": "servico", "quantity": 1, "unitPrice": 10},
		"moeda desconhecida": {"name": "X", "kind": "software", "quantity": 1, "unitPrice": 10, "currency": "EUR"},
	} {
		if got := e.call(owner, "POST", items, item, nil); got != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, got)
		}
	}
}

func TestMutationsRequireJSON(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/projects", strings.NewReader("clientName=x"))
	req.Header.Set("X-Test-User", owner)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("formulário de outro site: status %d", res.StatusCode)
	}
}

func TestDifalCalculatorAndHealth(t *testing.T) {
	e := newEnv(t)
	var res finance.DifalResult
	e.must(http.StatusOK, owner, "GET", "/api/difal?base=10000&origin=ES&imported=true", nil, &res)
	near(t, "DIFAL importado ES→SP", res.Value, 1400)
	e.must(http.StatusUnprocessableEntity, owner, "GET", "/api/difal?base=10000&origin=XX", nil, nil)

	var h map[string]string
	e.must(http.StatusOK, "", "GET", "/healthz", nil, &h)
	var me store.User
	e.must(http.StatusOK, admin, "GET", "/api/me", nil, &me)
	if me.Role != store.RoleAdmin {
		t.Errorf("papel = %s", me.Role)
	}
	e.must(http.StatusOK, owner, "GET", "/api/me", nil, &me)
	if me.Role != store.RoleUser {
		t.Errorf("papel = %s", me.Role)
	}
}
