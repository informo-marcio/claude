package finance

import (
	"errors"
	"fmt"
)

// ItemKind é a natureza de custo de um item da proposta.
type ItemKind string

const (
	// KindHardware é equipamento comprado; entra no CAPEX e recebe DIFAL.
	KindHardware ItemKind = "hardware"
	// KindSoftware é licença paga antecipadamente; entra no CAPEX, sem DIFAL.
	KindSoftware ItemKind = "software"
	// KindPointsLicense é licença por pontos consumida durante o contrato;
	// não é CAPEX, vira custo mensal (ver PointsLicensing).
	KindPointsLicense ItemKind = "points_license"
)

// Currency é a moeda em que o preço unitário foi informado.
type Currency string

const (
	BRL Currency = "BRL"
	USD Currency = "USD"
)

// Item é um item de aquisição da proposta, guardado na moeda original.
type Item struct {
	Name      string   `json:"name"`
	Kind      ItemKind `json:"kind"`
	Quantity  int      `json:"quantity"`
	UnitPrice float64  `json:"unitPrice"`
	Currency  Currency `json:"currency"`
	// OriginState é a UF do fornecedor (somente hardware).
	OriginState string `json:"originState,omitempty"`
	// Imported indica mercadoria importada (alíquota interestadual de 4%).
	Imported bool `json:"imported,omitempty"`
}

// ItemCost é o custo apurado de um item em reais.
type ItemCost struct {
	Name         string       `json:"name"`
	Kind         ItemKind     `json:"kind"`
	UnitPriceBRL float64      `json:"unitPriceBrl"`
	BaseBRL      float64      `json:"baseBrl"`
	Difal        *DifalResult `json:"difal,omitempty"`
	TotalBRL     float64      `json:"totalBrl"`
}

// Capex é a composição do investimento inicial de uma proposta.
type Capex struct {
	Hardware float64    `json:"hardware"`
	Software float64    `json:"software"`
	Difal    float64    `json:"difal"`
	Total    float64    `json:"total"`
	Items    []ItemCost `json:"items"`
}

// CapexParams são as premissas de apuração do CAPEX.
type CapexParams struct {
	// ExchangeRate é a cotação USD/BRL da proposta; obrigatória se houver
	// item em dólar.
	ExchangeRate float64
	// DestState é a UF onde a Informo recebe o equipamento.
	DestState string
	// InternalICMS são as alíquotas internas por UF.
	InternalICMS map[string]float64
}

// ComputeCapex apura o CAPEX a partir dos itens na moeda original.
//
// A conversão acontece aqui, a cada cálculo: mudar o câmbio da proposta
// reconverte todos os itens em dólar e o DIFAL correspondente. No portal
// anterior o valor convertido ficava gravado no item e não acompanhava a
// mudança de câmbio.
func ComputeCapex(items []Item, p CapexParams) (Capex, error) {
	out := Capex{Items: make([]ItemCost, 0, len(items))}
	var errs []error
	for i, it := range items {
		label := it.Name
		if label == "" {
			label = fmt.Sprintf("item %d", i+1)
		}
		if it.Kind == KindPointsLicense {
			continue
		}
		if it.Kind != KindHardware && it.Kind != KindSoftware {
			errs = append(errs, fmt.Errorf("%s: tipo de item desconhecido %q", label, it.Kind))
			continue
		}
		if it.Quantity < 1 {
			errs = append(errs, fmt.Errorf("%s: quantidade deve ser ao menos 1", label))
			continue
		}
		if !isAmount(it.UnitPrice) {
			errs = append(errs, fmt.Errorf("%s: preço unitário inválido", label))
			continue
		}
		unit := it.UnitPrice
		switch it.Currency {
		case BRL:
		case USD:
			if !isAmount(p.ExchangeRate) || p.ExchangeRate <= 0 {
				errs = append(errs, fmt.Errorf("%s: item em dólar exige câmbio da proposta", label))
				continue
			}
			unit = it.UnitPrice * p.ExchangeRate
		default:
			errs = append(errs, fmt.Errorf("%s: moeda desconhecida %q", label, it.Currency))
			continue
		}
		cost := ItemCost{Name: it.Name, Kind: it.Kind, UnitPriceBRL: roundCents(unit)}
		base := unit * float64(it.Quantity)
		cost.BaseBRL = roundCents(base)
		cost.TotalBRL = cost.BaseBRL
		if it.Kind == KindHardware {
			d, err := Difal(base, it.OriginState, p.DestState, it.Imported, p.InternalICMS)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", label, err))
				continue
			}
			cost.Difal = &d
			cost.TotalBRL = roundCents(cost.BaseBRL + d.Value)
			out.Hardware += cost.BaseBRL
			out.Difal += d.Value
		} else {
			out.Software += cost.BaseBRL
		}
		out.Items = append(out.Items, cost)
	}
	if err := errors.Join(errs...); err != nil {
		return Capex{}, err
	}
	out.Hardware = roundCents(out.Hardware)
	out.Software = roundCents(out.Software)
	out.Difal = roundCents(out.Difal)
	out.Total = roundCents(out.Hardware + out.Software + out.Difal)
	return out, nil
}
