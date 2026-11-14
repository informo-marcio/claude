// Package finance é o motor financeiro do portal de payback da Informo.
//
// Todas as funções são puras e determinísticas: recebem os insumos e a
// política explicitamente e não leem banco, relógio ou configuração global.
// Tela, PDF e Excel devem consumir somente os resultados deste pacote.
//
// Unidades: taxas são frações decimais (0.016 = 1,60% a.m.), inclusive a TIR.
// Valores monetários são float64 em reais, arredondados a centavos apenas na
// saída. Ponteiro nil significa "indisponível", nunca zero.
package finance

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// CommissionRates são as taxas de comissão de uma faixa de prazo.
type CommissionRates struct {
	Seller    float64 `json:"sellerRate"`
	Architect float64 `json:"architectRate"`
	Manager   float64 `json:"managerRate"`
}

// Total devolve a soma das três taxas.
func (r CommissionRates) Total() float64 { return r.Seller + r.Architect + r.Manager }

// Settings é a política financeira global, administrada pelo portal.
type Settings struct {
	// TaxRate é a carga agregada estimada sobre a receita (rótulo IBS+CBS).
	// É uma premissa de simulação, não apuração fiscal.
	TaxRate float64 `json:"taxRate"`
	// OpportunityRate é o rendimento mensal alternativo do caixa.
	OpportunityRate float64 `json:"monthlyOpportunityRate"`
	// IRRAlertThreshold é a TIR mensal mínima antes do alerta.
	IRRAlertThreshold float64 `json:"irrAlertThreshold"`
	// Durations são os prazos de contrato oferecidos, em meses.
	Durations []int `json:"contractDurations"`
	// MaxPayback é o mês máximo de payback descontado por prazo.
	MaxPayback map[int]int `json:"maxPaybackByDuration"`
	// Commission são as taxas de comissão por prazo.
	Commission map[int]CommissionRates `json:"commissionRatesByDuration"`
	// DefaultLossRate é a inadimplência esperada sugerida a novos projetos.
	DefaultLossRate float64 `json:"defaultExpectedLossRate"`
	// DefaultSupportCost é o custo mensal de suporte sugerido a novos projetos.
	DefaultSupportCost float64 `json:"defaultMonthlySupportCost"`
}

// MaxDurationMonths é o maior prazo de contrato aceito pelo motor.
const MaxDurationMonths = 120

// CurrentPolicy devolve a política vigente da Informo exportada do portal
// anterior em 03/11/2026. Serve de carga inicial do banco; depois disso a
// política persistida é a fonte de verdade.
func CurrentPolicy() Settings {
	return Settings{
		TaxRate:           0.265,
		OpportunityRate:   0.011,
		IRRAlertThreshold: 0.016,
		Durations:         []int{12, 24, 36, 48, 60},
		MaxPayback:        map[int]int{12: 11, 24: 16, 36: 24, 48: 33, 60: 41},
		Commission: map[int]CommissionRates{
			12: {Seller: 0.07, Architect: 0.037, Manager: 0.03},
			24: {Seller: 0.06, Architect: 0.032, Manager: 0.0257},
			36: {Seller: 0.05, Architect: 0.027, Manager: 0.0214},
			48: {Seller: 0.045, Architect: 0.024, Manager: 0.0184},
			60: {Seller: 0.04, Architect: 0.021, Manager: 0.0154},
		},
		DefaultLossRate:    0.003,
		DefaultSupportCost: 0,
	}
}

func isRate(v float64) bool { return !math.IsNaN(v) && v >= 0 && v <= 1 }

func isAmount(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }

// Validate confere se a política é coerente. Uma política inválida nunca é
// corrigida em silêncio: o chamador recebe o erro.
func (s Settings) Validate() error {
	var errs []error
	if !isRate(s.TaxRate) {
		errs = append(errs, errors.New("alíquota de impostos deve estar entre 0 e 1"))
	}
	if !isRate(s.OpportunityRate) {
		errs = append(errs, errors.New("taxa de oportunidade deve estar entre 0 e 1"))
	}
	if !isRate(s.IRRAlertThreshold) {
		errs = append(errs, errors.New("limite de alerta de TIR deve estar entre 0 e 1"))
	}
	if !isRate(s.DefaultLossRate) {
		errs = append(errs, errors.New("inadimplência padrão deve estar entre 0 e 1"))
	}
	if !isAmount(s.DefaultSupportCost) {
		errs = append(errs, errors.New("suporte padrão deve ser um valor não negativo"))
	}
	if len(s.Durations) == 0 {
		errs = append(errs, errors.New("informe ao menos um prazo de contrato"))
	}
	seen := map[int]bool{}
	for _, d := range s.Durations {
		if d < 1 || d > MaxDurationMonths {
			errs = append(errs, fmt.Errorf("prazo %d fora do intervalo 1..%d", d, MaxDurationMonths))
			continue
		}
		if seen[d] {
			errs = append(errs, fmt.Errorf("prazo %d repetido", d))
			continue
		}
		seen[d] = true
		p, ok := s.MaxPayback[d]
		if !ok {
			errs = append(errs, fmt.Errorf("prazo %d sem payback máximo", d))
		} else if p < 1 || p > d {
			errs = append(errs, fmt.Errorf("payback máximo do prazo %d deve estar entre 1 e %d", d, d))
		}
		r, ok := s.Commission[d]
		if !ok {
			errs = append(errs, fmt.Errorf("prazo %d sem taxas de comissão", d))
		} else if !isRate(r.Seller) || !isRate(r.Architect) || !isRate(r.Manager) || r.Total() > 1 {
			errs = append(errs, fmt.Errorf("comissões do prazo %d devem estar entre 0 e 1 e somar no máximo 100%%", d))
		}
	}
	return errors.Join(errs...)
}

// SortedDurations devolve os prazos em ordem crescente, sem alterar a política.
func (s Settings) SortedDurations() []int {
	out := append([]int(nil), s.Durations...)
	sort.Ints(out)
	return out
}

func (s Settings) hasDuration(d int) bool {
	for _, v := range s.Durations {
		if v == d {
			return true
		}
	}
	return false
}
