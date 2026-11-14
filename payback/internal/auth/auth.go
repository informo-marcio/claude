// Package auth define como o portal descobre quem fez uma requisição.
//
// A API depende só da interface Authenticator. A implementação de produção
// (Microsoft Entra ID) entra em um arquivo próprio; trocar por Cloudflare
// Access depois não exige mexer na API.
package auth

import (
	"errors"
	"net/http"
)

// Identity é quem o provedor de identidade diz que o usuário é.
type Identity struct {
	// Subject é o identificador estável no provedor (oid do Entra ID).
	Subject string
	Email   string
	Name    string
}

// ErrUnauthenticated indica requisição sem sessão válida.
var ErrUnauthenticated = errors.New("não autenticado")

// Authenticator identifica o autor de uma requisição.
type Authenticator interface {
	Authenticate(r *http.Request) (Identity, error)
}

// Fixed autentica toda requisição como uma única identidade. Existe só para
// desenvolvimento local e testes; o servidor se recusa a usá-lo fora de
// localhost.
type Fixed struct {
	Identity Identity
}

// Authenticate devolve sempre a identidade configurada.
func (f Fixed) Authenticate(*http.Request) (Identity, error) {
	if f.Identity.Email == "" {
		return Identity{}, ErrUnauthenticated
	}
	return f.Identity, nil
}
