package finance

import (
	"fmt"
	"strings"
)

// Alíquotas interestaduais de ICMS (Resolução do Senado 22/1989 e 13/2012).
const (
	interstateGeneral  = 0.12
	interstateReduced  = 0.07 // Sul/Sudeste (exceto ES) para Norte, Nordeste, Centro-Oeste e ES
	interstateImported = 0.04 // mercadoria importada ou com conteúdo de importação acima de 40%
)

// southSoutheastExceptES são as origens que usam 7% para as demais regiões.
var southSoutheastExceptES = map[string]bool{
	"MG": true, "PR": true, "RJ": true, "RS": true, "SC": true, "SP": true,
}

// DefaultInternalICMS são as alíquotas internas por UF herdadas do portal
// anterior. São premissas editáveis: confira com a contabilidade antes de
// usar em proposta, pois os estados as alteram com frequência.
func DefaultInternalICMS() map[string]float64 {
	return map[string]float64{
		"AC": 0.19, "AL": 0.19, "AM": 0.20, "AP": 0.18, "BA": 0.205, "CE": 0.20,
		"DF": 0.20, "ES": 0.17, "GO": 0.19, "MA": 0.23, "MT": 0.17, "MS": 0.17,
		"MG": 0.18, "PA": 0.19, "PB": 0.20, "PR": 0.195, "PE": 0.205, "PI": 0.225,
		"RJ": 0.20, "RN": 0.20, "RS": 0.17, "RO": 0.195, "RR": 0.20, "SC": 0.17,
		"SP": 0.18, "SE": 0.19, "TO": 0.20,
	}
}

// DifalResult é o DIFAL estimado de uma aquisição.
type DifalResult struct {
	InterstateRate float64 `json:"interstateRate"`
	InternalRate   float64 `json:"internalRate"`
	// Rate é a diferença aplicada sobre a base (nunca negativa).
	Rate  float64 `json:"rate"`
	Value float64 `json:"value"`
}

func normalizeUF(uf string) string { return strings.ToUpper(strings.TrimSpace(uf)) }

// InterstateRate devolve a alíquota interestadual entre duas UFs.
//
// O portal anterior aplicava 7% a toda origem fora do Sul/Sudeste com destino
// a São Paulo. A regra é a inversa: 7% vale quando a origem é Sul/Sudeste
// (exceto ES) e o destino é Norte, Nordeste, Centro-Oeste ou ES; nos demais
// casos é 12%, e 4% para mercadoria importada.
func InterstateRate(origin, dest string, imported bool, internal map[string]float64) (float64, error) {
	o, d := normalizeUF(origin), normalizeUF(dest)
	if _, ok := internal[o]; !ok {
		return 0, fmt.Errorf("UF de origem desconhecida: %q", origin)
	}
	if _, ok := internal[d]; !ok {
		return 0, fmt.Errorf("UF de destino desconhecida: %q", dest)
	}
	switch {
	case o == d:
		return 0, nil
	case imported:
		return interstateImported, nil
	case southSoutheastExceptES[o] && !southSoutheastExceptES[d]:
		return interstateReduced, nil
	default:
		return interstateGeneral, nil
	}
}

// Difal estima o diferencial de alíquota de uma compra interestadual pela
// diferença simples entre a alíquota interna do destino e a interestadual,
// aplicada sobre o valor da compra.
//
// É uma estimativa para a análise de viabilidade: não considera base dupla
// ("por dentro"), FCP, benefícios fiscais, NCM ou créditos.
func Difal(base float64, origin, dest string, imported bool, internal map[string]float64) (DifalResult, error) {
	if !isAmount(base) {
		return DifalResult{}, fmt.Errorf("base de DIFAL inválida: %v", base)
	}
	inter, err := InterstateRate(origin, dest, imported, internal)
	if err != nil {
		return DifalResult{}, err
	}
	o, d := normalizeUF(origin), normalizeUF(dest)
	if o == d {
		return DifalResult{InternalRate: internal[d]}, nil
	}
	rate := internal[d] - inter
	if rate < 0 {
		rate = 0
	}
	return DifalResult{
		InterstateRate: inter,
		InternalRate:   internal[d],
		Rate:           roundRate(rate),
		Value:          roundCents(base * rate),
	}, nil
}
