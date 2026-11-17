// Comando server sobe a API do portal de payback.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/informo-marcio/claude/payback/internal/api"
	"github.com/informo-marcio/claude/payback/internal/auth"
	"github.com/informo-marcio/claude/payback/internal/store"
	"github.com/informo-marcio/claude/payback/internal/webui"
)

// version é preenchida na compilação (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	// "payback healthcheck" consulta o próprio servidor e sai com 0 ou 1. A
	// imagem não tem shell nem curl, então a verificação de saúde do contêiner
	// usa o próprio binário.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("servidor encerrado com erro", "err", err)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("defina DATABASE_URL")
	}
	addr := env("LISTEN_ADDR", "127.0.0.1:8080")

	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}
	if err := st.SeedSettings(ctx); err != nil {
		return err
	}

	authenticator, routes, err := chooseAuthenticator(ctx, addr, st, log)
	if err != nil {
		return err
	}
	// Tudo que não é API nem login é a interface.
	routes = append(routes, func(mux *http.ServeMux) { mux.Handle("/", webui.Handler()) })

	admins := strings.Split(os.Getenv("ADMIN_EMAILS"), ",")
	srv := &http.Server{
		Addr:              addr,
		Handler:           webui.SecurityHeaders(api.New(st, authenticator, admins, log).Handler(routes...)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// Falha na hora se a porta estiver ocupada, em vez de procurar outra.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx) //nolint:errcheck // encerrando
	}()
	log.Info("portal no ar", "addr", addr, "versao", version)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck consulta /healthz no endereço local em que o servidor escuta.
func healthcheck() int {
	_, port, err := net.SplitHostPort(env("LISTEN_ADDR", "127.0.0.1:8080"))
	if err != nil {
		return 1
	}
	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// chooseAuthenticator escolhe como identificar o usuário. Com as variáveis
// ENTRA_* o login é pelo Entra ID. Sem elas, o servidor só sobe em modo de
// desenvolvimento (DEV_USER_EMAIL), e esse modo só é aceito em localhost.
func chooseAuthenticator(ctx context.Context, addr string, st *store.Store, log *slog.Logger) (auth.Authenticator, []func(*http.ServeMux), error) {
	tenant, clientID, secret := os.Getenv("ENTRA_TENANT_ID"), os.Getenv("ENTRA_CLIENT_ID"), os.Getenv("ENTRA_CLIENT_SECRET")
	devEmail := os.Getenv("DEV_USER_EMAIL")

	if tenant != "" || clientID != "" || secret != "" {
		if devEmail != "" {
			return nil, nil, errors.New("DEV_USER_EMAIL não pode ser usado junto com o login pelo Entra ID")
		}
		if tenant == "" || clientID == "" || secret == "" {
			return nil, nil, errors.New("defina ENTRA_TENANT_ID, ENTRA_CLIENT_ID e ENTRA_CLIENT_SECRET")
		}
		hours, err := strconv.Atoi(env("SESSION_HOURS", "12"))
		if err != nil || hours < 1 || hours > 168 {
			return nil, nil, errors.New("SESSION_HOURS deve estar entre 1 e 168")
		}
		o, err := auth.NewOIDC(ctx, auth.OIDCConfig{
			Issuer: auth.EntraIssuer(tenant), ClientID: clientID, ClientSecret: secret, TenantID: tenant,
			PublicURL: env("PUBLIC_URL", "http://localhost:8080"), SessionTTL: time.Duration(hours) * time.Hour,
		}, st, log)
		if err != nil {
			return nil, nil, err
		}
		log.Info("login pelo Entra ID habilitado", "tenant", tenant)
		return o, []func(*http.ServeMux){o.Register}, nil
	}

	if devEmail == "" {
		return nil, nil, errors.New("configure o login (ENTRA_TENANT_ID, ENTRA_CLIENT_ID, ENTRA_CLIENT_SECRET) ou, só para desenvolvimento local, DEV_USER_EMAIL")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, nil, err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, nil, errors.New("DEV_USER_EMAIL só é aceito com LISTEN_ADDR em localhost")
	}
	log.Warn("MODO DE DESENVOLVIMENTO: sem login, todas as requisições entram como " + devEmail)
	return auth.Fixed{Identity: auth.Identity{Email: devEmail, Name: env("DEV_USER_NAME", devEmail)}}, nil, nil
}
