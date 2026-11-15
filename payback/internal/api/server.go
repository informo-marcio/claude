// Package api expõe o portal como API JSON sob /api.
//
// Toda rota autentica o usuário e confere, no servidor, a permissão sobre o
// projeto envolvido. A interface nunca é a barreira de segurança.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/informo-marcio/claude/payback/internal/auth"
	"github.com/informo-marcio/claude/payback/internal/store"
)

// maxBodyBytes limita o corpo das requisições.
const maxBodyBytes = 1 << 20

// Server reúne as dependências das rotas.
type Server struct {
	store  *store.Store
	auth   auth.Authenticator
	admins map[string]bool
	log    *slog.Logger
}

// New cria o servidor. adminEmails são promovidos a administrador ao entrar.
func New(st *store.Store, a auth.Authenticator, adminEmails []string, log *slog.Logger) *Server {
	admins := map[string]bool{}
	for _, e := range adminEmails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			admins[e] = true
		}
	}
	return &Server{store: st, auth: a, admins: admins, log: log}
}

// apiError é um erro com status HTTP e mensagem para o usuário.
type apiError struct {
	Status  int      `json:"-"`
	Message string   `json:"error"`
	Details []string `json:"details,omitempty"`
}

func (e *apiError) Error() string { return e.Message }

func badRequest(msg string, details ...string) error {
	return &apiError{Status: http.StatusUnprocessableEntity, Message: msg, Details: details}
}

func forbidden(msg string) error { return &apiError{Status: http.StatusForbidden, Message: msg} }

var errNotFound = &apiError{Status: http.StatusNotFound, Message: "Registro não encontrado."}

type handler func(w http.ResponseWriter, r *http.Request, u store.User) error

// Handler devolve o roteador da API. Rotas extras (as de login) são
// registradas por extra, fora da autenticação.
func (s *Server) Handler(extra ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	for _, register := range extra {
		register(mux)
	}

	route := func(pattern string, h handler) { mux.HandleFunc(pattern, s.wrap(h)) }
	route("GET /api/me", s.me)
	route("GET /api/dashboard", s.dashboard)
	route("GET /api/difal", s.difal)
	route("GET /api/audit", s.auditLog)

	route("GET /api/settings", s.getSettings)
	route("PUT /api/settings", s.putSettings)
	route("GET /api/catalog", s.getCatalog)
	route("PUT /api/catalog/programs/{key}", s.putProgram)
	route("PUT /api/catalog/models/{id}", s.putModel)

	route("GET /api/projects", s.listProjects)
	route("POST /api/projects", s.createProject)
	route("GET /api/projects/{id}", s.getProject)
	route("PUT /api/projects/{id}", s.updateProject)
	route("DELETE /api/projects/{id}", s.deleteProject)
	route("GET /api/projects/{id}/events", s.listEvents)

	route("POST /api/projects/{id}/items", s.createItem)
	route("PUT /api/projects/{id}/items/{itemId}", s.updateItem)
	route("DELETE /api/projects/{id}/items/{itemId}", s.deleteItem)
	route("POST /api/projects/{id}/items/{itemId}/duplicate", s.duplicateItem)

	route("GET /api/projects/{id}/shares", s.listShares)
	route("POST /api/projects/{id}/shares", s.addShare)
	route("DELETE /api/projects/{id}/shares/{userId}", s.removeShare)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "banco indisponível"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// wrap autentica, aplica as proteções comuns e traduz erros em respostas.
func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Exigir JSON impede que um formulário de outro site dispare
			// alterações com a sessão do usuário (CSRF).
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
				s.fail(w, r, &apiError{Status: http.StatusUnsupportedMediaType, Message: "Envie o corpo como application/json."})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		id, err := s.auth.Authenticate(r)
		if err != nil {
			s.fail(w, r, &apiError{Status: http.StatusUnauthorized, Message: "Sessão expirada. Entre novamente."})
			return
		}
		u, err := s.store.UpsertUser(r.Context(), id.Subject, id.Email, id.Name, s.admins[strings.ToLower(id.Email)])
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := h(w, r, u); err != nil {
			s.fail(w, r, err)
		}
	}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &ae):
	case errors.Is(err, store.ErrNotFound):
		ae = errNotFound
	case errors.Is(err, store.ErrConflict):
		ae = &apiError{Status: http.StatusConflict, Message: store.ErrConflict.Error()}
	case errors.As(err, &tooLarge):
		ae = &apiError{Status: http.StatusRequestEntityTooLarge, Message: "Requisição grande demais."}
	case errors.Is(err, context.Canceled):
		return
	default:
		// O detalhe fica no log; o usuário não recebe erro interno do banco.
		s.log.Error("erro interno", "method", r.Method, "path", r.URL.Path, "err", err)
		ae = &apiError{Status: http.StatusInternalServerError, Message: "Erro interno. Tente novamente."}
	}
	writeJSON(w, ae.Status, ae)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // cliente desconectado
}

// readJSON decodifica o corpo, recusando campos desconhecidos.
func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return err
		}
		return &apiError{Status: http.StatusBadRequest, Message: "JSON inválido: " + err.Error()}
	}
	if dec.More() {
		return &apiError{Status: http.StatusBadRequest, Message: "JSON inválido: conteúdo após o objeto."}
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id < 1 {
		return 0, errNotFound
	}
	return id, nil
}

// Níveis de acesso a um projeto.
const (
	accessOwner  = "owner"
	accessShared = "shared"
	accessAdmin  = "admin"
)

// projectFor carrega o projeto da rota e o nível de acesso do usuário. Quem
// não pode ler recebe "não encontrado", sem confirmar que o projeto existe.
func (s *Server) projectFor(r *http.Request, u store.User) (store.Project, string, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return store.Project{}, "", err
	}
	p, err := s.store.Project(r.Context(), id)
	if err != nil {
		return store.Project{}, "", err
	}
	if p.OwnerID == u.ID {
		return p, accessOwner, nil
	}
	shared, err := s.store.IsSharedWith(r.Context(), id, u.ID)
	if err != nil {
		return store.Project{}, "", err
	}
	switch {
	case u.IsAdmin():
		return p, accessAdmin, nil
	case shared:
		return p, accessShared, nil
	}
	return store.Project{}, "", errNotFound
}

// editableProject exige permissão de alteração: o dono ou um administrador.
// Compartilhamento concede só leitura. O Actor devolvido marca quando é o
// administrador agindo em projeto alheio, e vai para a trilha de auditoria.
func (s *Server) editableProject(r *http.Request, u store.User) (store.Project, string, store.Actor, error) {
	p, access, err := s.projectFor(r, u)
	if err != nil {
		return p, access, store.Actor{}, err
	}
	if access != accessOwner && access != accessAdmin {
		return p, access, store.Actor{}, forbidden("Somente o dono do projeto ou um administrador pode alterá-lo.")
	}
	return p, access, actorFor(u, access == accessAdmin), nil
}

func actorFor(u store.User, asAdmin bool) store.Actor {
	return store.Actor{UserID: u.ID, Email: u.Email, Name: u.Name, AsAdmin: asAdmin}
}

func requireAdmin(u store.User) error {
	if !u.IsAdmin() {
		return forbidden("Acesso restrito a administradores.")
	}
	return nil
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, u store.User) error {
	writeJSON(w, http.StatusOK, u)
	return nil
}

func money(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return fmt.Sprintf("R$ %s,%s", b.String(), frac)
}
