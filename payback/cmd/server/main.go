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
	"strings"
	"syscall"
	"time"

	"github.com/informo-marcio/claude/payback/internal/api"
	"github.com/informo-marcio/claude/payback/internal/auth"
	"github.com/informo-marcio/claude/payback/internal/store"
)

func main() {
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

	authenticator, err := chooseAuthenticator(addr, log)
	if err != nil {
		return err
	}

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

	admins := strings.Split(os.Getenv("ADMIN_EMAILS"), ",")
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(st, authenticator, admins, log).Handler(),
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
	log.Info("portal no ar", "addr", addr)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// chooseAuthenticator escolhe como identificar o usuário. O login pelo Entra
// ID ainda não foi implementado; até lá o servidor só sobe em modo de
// desenvolvimento, e esse modo só é aceito escutando em localhost.
func chooseAuthenticator(addr string, log *slog.Logger) (auth.Authenticator, error) {
	devEmail := os.Getenv("DEV_USER_EMAIL")
	if devEmail == "" {
		return nil, errors.New("login pelo Entra ID ainda não implementado; para desenvolvimento local defina DEV_USER_EMAIL")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("DEV_USER_EMAIL só é aceito com LISTEN_ADDR em localhost")
	}
	log.Warn("MODO DE DESENVOLVIMENTO: sem login, todas as requisições entram como " + devEmail)
	return auth.Fixed{Identity: auth.Identity{Email: devEmail, Name: env("DEV_USER_NAME", devEmail)}}, nil
}
