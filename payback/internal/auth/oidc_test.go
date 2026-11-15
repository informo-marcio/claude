package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	testClient = "cliente-de-teste"
	testSecret = "segredo-de-teste"
	testTenant = "locatario-informo"
)

// fakeIdP é um provedor OpenID Connect mínimo: descoberta, chaves e troca de
// código por token. Confere o segredo do cliente e o PKCE, como o Entra faz.
type fakeIdP struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	signer *rsa.PrivateKey // chave que assina; difere de key para simular fraude

	mu     sync.Mutex
	codes  map[string]pendingCode
	mutate func(claims map[string]any)
}

type pendingCode struct {
	challenge string
	nonce     string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{t: t, key: key, signer: key, codes: map[string]pendingCode{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.srv.URL,
			"authorization_endpoint":                idp.srv.URL + "/authorize",
			"token_endpoint":                        idp.srv.URL + "/token",
			"jwks_uri":                              idp.srv.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", idp.token)
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

// authorize faz o papel do usuário se autenticando no provedor: recebe a URL
// para onde o portal o mandou e devolve o código de autorização.
func (i *fakeIdP) authorize(authURL string) (code, state string) {
	u, err := url.Parse(authURL)
	if err != nil {
		i.t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != testClient || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
		q.Get("code_challenge") == "" || q.Get("nonce") == "" || !strings.Contains(q.Get("scope"), "openid") {
		i.t.Fatalf("pedido de autorização incompleto: %s", authURL)
	}
	code = "code-" + q.Get("state")[:8]
	i.mu.Lock()
	i.codes[code] = pendingCode{challenge: q.Get("code_challenge"), nonce: q.Get("nonce")}
	i.mu.Unlock()
	return code, q.Get("state")
}

func (i *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm() //nolint:errcheck
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.Form.Get("client_id"), r.Form.Get("client_secret")
	}
	i.mu.Lock()
	pending, ok := i.codes[r.Form.Get("code")]
	delete(i.codes, r.Form.Get("code"))
	mutate := i.mutate
	i.mu.Unlock()
	sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if id != testClient || secret != testSecret || !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != pending.challenge {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
		return
	}
	claims := map[string]any{
		"iss": i.srv.URL, "aud": testClient, "sub": "sub-123", "oid": "oid-123", "tid": testTenant,
		"preferred_username": "Marcio@Informo.com.br", "name": "Marcio Talhacolo", "nonce": pending.nonce,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
	if mutate != nil {
		mutate(claims)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: i.signer, KeyID: "k1"}}, nil)
	if err != nil {
		i.t.Error(err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		i.t.Error(err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
}

// memSessions é um SessionStore em memória com relógio controlável.
type memSessions struct {
	mu   sync.Mutex
	now  func() time.Time
	rows map[string]memSession
}

type memSession struct {
	id      Identity
	expires time.Time
}

func (m *memSessions) CreateSession(_ context.Context, hash []byte, id Identity, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[string(hash)] = memSession{id, expires}
	return nil
}

func (m *memSessions) SessionIdentity(_ context.Context, hash []byte) (Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.rows[string(hash)]
	if !ok || !m.now().Before(s.expires) {
		return Identity{}, ErrUnauthenticated
	}
	return s.id, nil
}

func (m *memSessions) DeleteSession(_ context.Context, hash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, string(hash))
	return nil
}

type portal struct {
	t        *testing.T
	idp      *fakeIdP
	srv      *httptest.Server
	sessions *memSessions
	client   *http.Client
}

func newPortal(t *testing.T) *portal {
	t.Helper()
	idp := newFakeIdP(t)
	sessions := &memSessions{now: time.Now, rows: map[string]memSession{}}
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	o, err := NewOIDC(context.Background(), OIDCConfig{
		Issuer: idp.srv.URL, ClientID: testClient, ClientSecret: testSecret, TenantID: testTenant,
		PublicURL: srv.URL, SessionTTL: time.Hour,
	}, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	o.Register(mux)
	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		id, err := o.Authenticate(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"email": id.Email, "subject": id.Subject, "name": id.Name})
	})
	srv.Config.Handler = mux

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &portal{t: t, idp: idp, srv: srv, sessions: sessions, client: client}
}

func (p *portal) get(path string) *http.Response {
	p.t.Helper()
	res, err := p.client.Get(p.srv.URL + path)
	if err != nil {
		p.t.Fatal(err)
	}
	res.Body.Close()
	return res
}

// signIn percorre o login completo e devolve a resposta do callback.
func (p *portal) signIn(loginPath string) *http.Response {
	p.t.Helper()
	res := p.get(loginPath)
	if res.StatusCode != http.StatusFound {
		p.t.Fatalf("login: status %d", res.StatusCode)
	}
	code, state := p.idp.authorize(res.Header.Get("Location"))
	return p.get("/auth/callback?code=" + code + "&state=" + state)
}

func (p *portal) whoami() int { return p.get("/whoami").StatusCode }

func TestLoginCreatesServerSideSession(t *testing.T) {
	p := newPortal(t)
	if p.whoami() != http.StatusUnauthorized {
		t.Fatal("sem sessão o portal deveria recusar")
	}
	res := p.signIn("/auth/login?returnTo=/projects/7")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/projects/7" {
		t.Fatalf("callback: status %d, destino %q", res.StatusCode, res.Header.Get("Location"))
	}
	var session *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "payback_session" {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.MaxAge != 3600 {
		t.Fatalf("cookie de sessão: %+v", session)
	}

	res, err := p.client.Get(p.srv.URL + "/whoami")
	if err != nil {
		t.Fatal(err)
	}
	var who map[string]string
	json.NewDecoder(res.Body).Decode(&who) //nolint:errcheck
	res.Body.Close()
	if who["email"] != "marcio@informo.com.br" || who["subject"] != "oid-123" || who["name"] != "Marcio Talhacolo" {
		t.Errorf("identidade: %v", who)
	}

	// O banco guarda só o hash: o valor do cookie não aparece nas sessões.
	for hash := range p.sessions.rows {
		if strings.Contains(hash, session.Value) {
			t.Error("o token da sessão foi guardado em claro")
		}
	}
}

func TestLogoutRevokesSessionOnServer(t *testing.T) {
	p := newPortal(t)
	res := p.signIn("/auth/login")
	var stolen *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "payback_session" {
			stolen = c
		}
	}
	post := func(contentType string) int {
		req, _ := http.NewRequest("POST", p.srv.URL+"/auth/logout", strings.NewReader("{}"))
		req.Header.Set("Content-Type", contentType)
		res, err := p.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	// Um formulário de outro site não consegue deslogar o usuário.
	if got := post("application/x-www-form-urlencoded"); got != http.StatusUnsupportedMediaType {
		t.Errorf("logout sem JSON: status %d", got)
	}
	if p.whoami() != http.StatusOK {
		t.Fatal("a sessão deveria continuar válida")
	}
	if got := post("application/json"); got != http.StatusNoContent {
		t.Fatalf("logout: status %d", got)
	}
	// Reapresentar o cookie antigo, copiado antes do logout, não funciona.
	req, _ := http.NewRequest("GET", p.srv.URL+"/whoami", nil)
	req.AddCookie(stolen)
	replay, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusUnauthorized {
		t.Errorf("cookie antigo após logout: status %d", replay.StatusCode)
	}
}

func TestSessionExpires(t *testing.T) {
	p := newPortal(t)
	p.signIn("/auth/login")
	if p.whoami() != http.StatusOK {
		t.Fatal("sessão recém-criada deveria valer")
	}
	p.sessions.mu.Lock()
	p.sessions.now = func() time.Time { return time.Now().Add(61 * time.Minute) }
	p.sessions.mu.Unlock()
	if p.whoami() != http.StatusUnauthorized {
		t.Error("sessão vencida deveria ser recusada")
	}
}

func TestCallbackRejectsForgedOrMismatchedLogins(t *testing.T) {
	cases := map[string]struct {
		setup func(p *portal)
		want  int
	}{
		"outro locatário":          {func(p *portal) { p.idp.mutate = func(c map[string]any) { c["tid"] = "outra-empresa" } }, http.StatusForbidden},
		"nonce trocado":            {func(p *portal) { p.idp.mutate = func(c map[string]any) { c["nonce"] = "outro" } }, http.StatusUnauthorized},
		"token para outro cliente": {func(p *portal) { p.idp.mutate = func(c map[string]any) { c["aud"] = "outro-app" } }, http.StatusUnauthorized},
		"token vencido": {func(p *portal) {
			p.idp.mutate = func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }
		}, http.StatusUnauthorized},
		"emissor falso":    {func(p *portal) { p.idp.mutate = func(c map[string]any) { c["iss"] = "https://falso.example" } }, http.StatusUnauthorized},
		"conta sem e-mail": {func(p *portal) { p.idp.mutate = func(c map[string]any) { delete(c, "preferred_username") } }, http.StatusForbidden},
		"assinatura de outra chave": {func(p *portal) {
			other, _ := rsa.GenerateKey(rand.Reader, 2048)
			p.idp.signer = other
		}, http.StatusUnauthorized},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPortal(t)
			c.setup(p)
			if got := p.signIn("/auth/login").StatusCode; got != c.want {
				t.Errorf("status %d, esperado %d", got, c.want)
			}
			if p.whoami() != http.StatusUnauthorized || len(p.sessions.rows) != 0 {
				t.Error("login recusado não pode criar sessão")
			}
		})
	}
}

func TestCallbackRequiresMatchingState(t *testing.T) {
	p := newPortal(t)
	res := p.get("/auth/login")
	code, _ := p.idp.authorize(res.Header.Get("Location"))
	if got := p.get("/auth/callback?code=" + code + "&state=forjado").StatusCode; got != http.StatusBadRequest {
		t.Errorf("state forjado: status %d", got)
	}
	// O cookie de login é de uso único: repetir o callback correto falha.
	res = p.get("/auth/login")
	code, state := p.idp.authorize(res.Header.Get("Location"))
	if got := p.get("/auth/callback?code=" + code + "&state=" + state).StatusCode; got != http.StatusFound {
		t.Fatalf("login legítimo: status %d", got)
	}
	if got := p.get("/auth/callback?code=" + code + "&state=" + state).StatusCode; got != http.StatusBadRequest {
		t.Errorf("callback repetido: status %d", got)
	}

	// Sem ter passado pelo /auth/login (navegador sem o cookie), não entra.
	fresh := newPortal(t)
	if got := fresh.get("/auth/callback?code=x&state=y").StatusCode; got != http.StatusBadRequest {
		t.Errorf("callback sem login iniciado: status %d", got)
	}
	if got := fresh.get("/auth/callback?error=access_denied&state=y").StatusCode; got != http.StatusUnauthorized {
		t.Errorf("erro do provedor: status %d", got)
	}
}

func TestReturnToIsNeverAnOpenRedirect(t *testing.T) {
	for in, want := range map[string]string{
		"/projects/7?tab=itens": "/projects/7?tab=itens",
		"":                      "/",
		"//evil.example":        "/",
		"https://evil.example":  "/",
		"/\\evil.example":       "/",
		"javascript:alert(1)":   "/",
	} {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %q, esperado %q", in, got, want)
		}
	}
	p := newPortal(t)
	if got := p.signIn("/auth/login?returnTo=" + url.QueryEscape("//evil.example")).Header.Get("Location"); got != "/" {
		t.Errorf("destino após login = %q", got)
	}
}

func TestOIDCConfigIsValidated(t *testing.T) {
	idp := newFakeIdP(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ok := OIDCConfig{Issuer: idp.srv.URL, ClientID: testClient, ClientSecret: testSecret, PublicURL: "https://payback.informo.com.br"}
	o, err := NewOIDC(context.Background(), ok, &memSessions{}, log)
	if err != nil {
		t.Fatal(err)
	}
	// Em HTTPS os cookies são Secure e usam o prefixo __Host-.
	if !o.secure || o.cookieName("payback_session") != "__Host-payback_session" {
		t.Errorf("cookies em HTTPS: secure=%v, nome=%s", o.secure, o.cookieName("payback_session"))
	}
	if o.oauth.RedirectURL != "https://payback.informo.com.br/auth/callback" {
		t.Errorf("redirect = %s", o.oauth.RedirectURL)
	}
	if got := EntraIssuer("abc"); got != "https://login.microsoftonline.com/abc/v2.0" {
		t.Errorf("emissor do Entra = %s", got)
	}

	bad := map[string]func(*OIDCConfig){
		"sem segredo":            func(c *OIDCConfig) { c.ClientSecret = "" },
		"http fora de localhost": func(c *OIDCConfig) { c.PublicURL = "http://payback.informo.com.br" },
		"URL inválida":           func(c *OIDCConfig) { c.PublicURL = "payback" },
		"emissor fora do ar":     func(c *OIDCConfig) { c.Issuer = "http://127.0.0.1:1" },
	}
	for name, change := range bad {
		cfg := ok
		change(&cfg)
		if _, err := NewOIDC(context.Background(), cfg, &memSessions{}, log); err == nil {
			t.Errorf("%s: esperado erro", name)
		}
	}
}
