package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// SessionStore guarda as sessões no servidor.
type SessionStore interface {
	CreateSession(ctx context.Context, tokenHash []byte, id Identity, expiresAt time.Time) error
	SessionIdentity(ctx context.Context, tokenHash []byte) (Identity, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

// OIDCConfig configura o login por OpenID Connect.
type OIDCConfig struct {
	// Issuer é o emissor; no Entra ID,
	// https://login.microsoftonline.com/<locatário>/v2.0.
	Issuer       string
	ClientID     string
	ClientSecret string
	// TenantID, quando informado, exige que o token venha desse locatário.
	TenantID string
	// PublicURL é o endereço do portal visto pelo navegador, sem barra final.
	PublicURL string
	// SessionTTL é a duração máxima de uma sessão.
	SessionTTL time.Duration
}

// EntraIssuer monta o emissor do Entra ID para um locatário.
func EntraIssuer(tenantID string) string {
	return "https://login.microsoftonline.com/" + tenantID + "/v2.0"
}

// OIDC faz o login pelo provedor de identidade e mantém a sessão por cookie.
// O fluxo é o de código de autorização com PKCE, state e nonce.
type OIDC struct {
	cfg      OIDCConfig
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	sessions SessionStore
	secure   bool
	log      *slog.Logger
}

const (
	loginCookieTTL = 10 * time.Minute
	callbackPath   = "/auth/callback"
)

// NewOIDC descobre o provedor e prepara o login. Falha se o emissor não
// responder: o portal não sobe sem conseguir validar tokens.
func NewOIDC(ctx context.Context, cfg OIDCConfig, sessions SessionStore, log *slog.Logger) (*OIDC, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("login: emissor, ID do cliente e segredo são obrigatórios")
	}
	base, err := url.Parse(cfg.PublicURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return nil, fmt.Errorf("login: PUBLIC_URL inválida: %q", cfg.PublicURL)
	}
	host := base.Hostname()
	if base.Scheme == "http" && host != "localhost" && host != "127.0.0.1" {
		return nil, errors.New("login: fora de localhost a PUBLIC_URL precisa ser https")
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("login: descoberta do provedor: %w", err)
	}
	return &OIDC{
		cfg: cfg,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.PublicURL + callbackPath,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		sessions: sessions,
		secure:   base.Scheme == "https",
		log:      log,
	}, nil
}

// Register adiciona as rotas de login ao roteador.
func (o *OIDC) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", o.login)
	mux.HandleFunc("GET "+callbackPath, o.callback)
	mux.HandleFunc("POST /auth/logout", o.logout)
}

// Com HTTPS os cookies usam o prefixo __Host-, que o navegador só aceita com
// Secure, Path=/ e sem Domain: um subdomínio não consegue plantá-los.
func (o *OIDC) cookieName(base string) string {
	if o.secure {
		return "__Host-" + base
	}
	return base
}

func (o *OIDC) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	c := &http.Cookie{
		Name: o.cookieName(name), Value: value, Path: "/",
		HttpOnly: true, Secure: o.secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(ttl.Seconds()),
	}
	if ttl <= 0 {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// loginState é o que o navegador guarda entre a ida ao provedor e a volta.
type loginState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
}

// safeReturnTo aceita só caminhos locais, para o login não virar
// redirecionador aberto.
func safeReturnTo(v string) string {
	if !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || strings.ContainsAny(v, "\\\r\n") {
		return "/"
	}
	return v
}

func (o *OIDC) login(w http.ResponseWriter, r *http.Request) {
	state, err1 := randomToken()
	nonce, err2 := randomToken()
	if err := errors.Join(err1, err2); err != nil {
		o.fail(w, r, http.StatusInternalServerError, "Não foi possível iniciar o login.", err)
		return
	}
	verifier := oauth2.GenerateVerifier()
	raw, _ := json.Marshal(loginState{State: state, Nonce: nonce, Verifier: verifier, ReturnTo: safeReturnTo(r.URL.Query().Get("returnTo"))})
	o.setCookie(w, "payback_login", base64.RawURLEncoding.EncodeToString(raw), loginCookieTTL)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (o *OIDC) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var st loginState
	if c, err := r.Cookie(o.cookieName("payback_login")); err == nil {
		if raw, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil {
			json.Unmarshal(raw, &st) //nolint:errcheck // estado vazio falha abaixo
		}
	}
	o.setCookie(w, "payback_login", "", 0)

	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		o.fail(w, r, http.StatusUnauthorized, "O provedor de identidade recusou o login.", fmt.Errorf("%s: %s", e, q.Get("error_description")))
		return
	}
	if st.State == "" || q.Get("state") != st.State {
		o.fail(w, r, http.StatusBadRequest, "Login expirado ou iniciado em outra aba. Tente de novo.", errors.New("state não confere"))
		return
	}
	token, err := o.oauth.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		o.fail(w, r, http.StatusUnauthorized, "Não foi possível concluir o login.", err)
		return
	}
	rawID, _ := token.Extra("id_token").(string)
	idToken, err := o.verifier.Verify(r.Context(), rawID)
	if err != nil {
		o.fail(w, r, http.StatusUnauthorized, "Não foi possível validar o login.", err)
		return
	}
	var claims struct {
		OID      string `json:"oid"`
		TID      string `json:"tid"`
		Email    string `json:"email"`
		Username string `json:"preferred_username"`
		Name     string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		o.fail(w, r, http.StatusUnauthorized, "Não foi possível validar o login.", err)
		return
	}
	if idToken.Nonce != st.Nonce {
		o.fail(w, r, http.StatusUnauthorized, "Não foi possível validar o login.", errors.New("nonce não confere"))
		return
	}
	if o.cfg.TenantID != "" && claims.TID != o.cfg.TenantID {
		o.fail(w, r, http.StatusForbidden, "Esta conta não pertence à organização.", fmt.Errorf("locatário %q", claims.TID))
		return
	}
	id := Identity{Subject: claims.OID, Email: strings.ToLower(strings.TrimSpace(claims.Email)), Name: claims.Name}
	if id.Subject == "" {
		id.Subject = idToken.Subject
	}
	if id.Email == "" {
		id.Email = strings.ToLower(strings.TrimSpace(claims.Username))
	}
	if !strings.Contains(id.Email, "@") {
		o.fail(w, r, http.StatusForbidden, "A conta não tem e-mail.", errors.New("token sem e-mail"))
		return
	}

	session, err := randomToken()
	if err != nil {
		o.fail(w, r, http.StatusInternalServerError, "Não foi possível criar a sessão.", err)
		return
	}
	if err := o.sessions.CreateSession(r.Context(), hashToken(session), id, time.Now().Add(o.cfg.SessionTTL)); err != nil {
		o.fail(w, r, http.StatusInternalServerError, "Não foi possível criar a sessão.", err)
		return
	}
	o.setCookie(w, "payback_session", session, o.cfg.SessionTTL)
	http.Redirect(w, r, safeReturnTo(st.ReturnTo), http.StatusFound)
}

// logout apaga a sessão no servidor: o cookie deixa de valer mesmo que tenha
// sido copiado. Exige JSON pelo mesmo motivo das outras alterações (CSRF).
func (o *OIDC) logout(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "Envie o corpo como application/json.", http.StatusUnsupportedMediaType)
		return
	}
	if c, err := r.Cookie(o.cookieName("payback_session")); err == nil {
		if err := o.sessions.DeleteSession(r.Context(), hashToken(c.Value)); err != nil {
			o.fail(w, r, http.StatusInternalServerError, "Não foi possível sair.", err)
			return
		}
	}
	o.setCookie(w, "payback_session", "", 0)
	w.WriteHeader(http.StatusNoContent)
}

// Authenticate identifica o usuário pela sessão do cookie.
func (o *OIDC) Authenticate(r *http.Request) (Identity, error) {
	c, err := r.Cookie(o.cookieName("payback_session"))
	if err != nil || c.Value == "" {
		return Identity{}, ErrUnauthenticated
	}
	id, err := o.sessions.SessionIdentity(r.Context(), hashToken(c.Value))
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}
	return id, nil
}

// fail responde com uma mensagem simples e deixa o detalhe só no log.
func (o *OIDC) fail(w http.ResponseWriter, r *http.Request, status int, msg string, err error) {
	o.log.Warn("login falhou", "path", r.URL.Path, "status", status, "err", err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, msg)
	fmt.Fprintln(w, "Voltar ao login: "+o.cfg.PublicURL+"/auth/login")
}
