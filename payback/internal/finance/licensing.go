package finance

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PointsLicense são as premissas de uma licença por pontos, congeladas no
// item da proposta no momento da inclusão. Mudanças posteriores no catálogo
// não alteram propostas existentes.
type PointsLicense struct {
	Name string `json:"name"`
	// DailyPoints é o consumo de pontos por dia e por unidade; nil enquanto
	// o catálogo não tiver o consumo cadastrado.
	DailyPoints *float64 `json:"dailyPoints"`
	// PriceUSD é o preço de um ponto em dólar.
	PriceUSD float64 `json:"priceUsd"`
	// ExchangeRate é a cotação USD/BRL congelada com o item.
	ExchangeRate float64 `json:"exchangeRate"`
	Quantity     int     `json:"quantity"`
}

// LicenseMonth é o consumo estimado de um mês de vigência.
type LicenseMonth struct {
	Month            int     `json:"month"`
	StartDate        string  `json:"startDate"`
	EndDateExclusive string  `json:"endDateExclusive"`
	Days             int     `json:"days"`
	Points           float64 `json:"points"`
	CostUSD          float64 `json:"costUsd"`
	CostBRL          float64 `json:"costBrl"`
}

// LicenseSchedule é a estimativa de consumo de uma licença no contrato.
// É uma estimativa econômica: não representa compra, saldo nem reposição de
// pacotes de pontos.
type LicenseSchedule struct {
	TotalDays    int            `json:"totalDays"`
	TotalPoints  float64        `json:"totalPoints"`
	TotalCostUSD float64        `json:"totalCostUsd"`
	TotalCostBRL float64        `json:"totalCostBrl"`
	Months       []LicenseMonth `json:"months"`
}

// ErrPendingConsumption indica licença sem consumo diário cadastrado. Nenhum
// custo pode ser estimado; nunca é tratado como custo zero.
var ErrPendingConsumption = errors.New("consumo diário de pontos não cadastrado")

const dateLayout = "2006-01-02"

func parseStartDate(value string) (time.Time, error) {
	d, err := time.Parse(dateLayout, value)
	if err != nil || d.Year() < 1900 || d.Year() > 9800 {
		return time.Time{}, fmt.Errorf("data inicial inválida: %q (use AAAA-MM-DD)", value)
	}
	return d, nil
}

// anniversary devolve a data de início do mês de vigência de índice offset,
// mantendo o dia original quando o mês o comporta (31/01 → 29/02 → 31/03).
func anniversary(start time.Time, offset int) time.Time {
	last := time.Date(start.Year(), start.Month()+time.Month(offset)+1, 0, 0, 0, 0, 0, time.UTC)
	day := start.Day()
	if last.Day() < day {
		day = last.Day()
	}
	return time.Date(last.Year(), last.Month(), day, 0, 0, 0, 0, time.UTC)
}

// EstimateLicense calcula o consumo mensal por dias corridos reais.
func EstimateLicense(l PointsLicense, durationMonths int, startDate string) (LicenseSchedule, error) {
	start, err := parseStartDate(startDate)
	if err != nil {
		return LicenseSchedule{}, err
	}
	if durationMonths < 1 || durationMonths > MaxDurationMonths {
		return LicenseSchedule{}, fmt.Errorf("prazo deve estar entre 1 e %d meses", MaxDurationMonths)
	}
	if l.Quantity < 1 || l.Quantity > 10000 {
		return LicenseSchedule{}, errors.New("quantidade deve estar entre 1 e 10.000")
	}
	if !isAmount(l.PriceUSD) || l.PriceUSD <= 0 || l.PriceUSD > 999999 {
		return LicenseSchedule{}, errors.New("preço do ponto inválido")
	}
	if !isAmount(l.ExchangeRate) || l.ExchangeRate <= 0 || l.ExchangeRate > 10000 {
		return LicenseSchedule{}, errors.New("câmbio inválido")
	}
	if l.DailyPoints == nil {
		return LicenseSchedule{}, ErrPendingConsumption
	}
	daily := *l.DailyPoints
	if !isAmount(daily) || daily <= 0 || daily > 999999 {
		return LicenseSchedule{}, errors.New("consumo diário inválido")
	}
	out := LicenseSchedule{Months: make([]LicenseMonth, durationMonths)}
	for i := 0; i < durationMonths; i++ {
		from, until := anniversary(start, i), anniversary(start, i+1)
		days := int(until.Sub(from).Hours()/24 + 0.5)
		points := daily * float64(l.Quantity) * float64(days)
		row := LicenseMonth{
			Month:            i + 1,
			StartDate:        from.Format(dateLayout),
			EndDateExclusive: until.Format(dateLayout),
			Days:             days,
			Points:           roundTo(points, 6),
			CostUSD:          roundCents(points * l.PriceUSD),
			CostBRL:          roundCents(points * l.PriceUSD * l.ExchangeRate),
		}
		out.Months[i] = row
		out.TotalDays += days
		out.TotalPoints += row.Points
		out.TotalCostUSD += row.CostUSD
		out.TotalCostBRL += row.CostBRL
	}
	out.TotalPoints = roundTo(out.TotalPoints, 6)
	out.TotalCostUSD = roundCents(out.TotalCostUSD)
	out.TotalCostBRL = roundCents(out.TotalCostBRL)
	return out, nil
}

// PointsLicensing é o custo recorrente de todas as licenças por pontos de uma
// proposta.
type PointsLicensing struct {
	TotalCostBRL float64 `json:"totalCostBrl"`
	// MonthlyCosts tem uma posição por mês do contrato; é o insumo
	// Input.LicensingCosts do motor.
	MonthlyCosts []float64         `json:"monthlyCosts"`
	Schedules    []LicenseSchedule `json:"schedules"`
}

// IncompleteLicensingError lista as licenças que impedem o cálculo. Enquanto
// houver pendência a proposta não tem resultado financeiro.
type IncompleteLicensingError struct {
	Items []string
}

func (e *IncompleteLicensingError) Error() string {
	return "licenças por pontos com premissas incompletas: " + strings.Join(e.Items, ", ")
}

// EstimateProjectLicensing soma o custo mensal de todas as licenças.
func EstimateProjectLicensing(licenses []PointsLicense, durationMonths int, startDate string) (PointsLicensing, error) {
	if durationMonths < 1 || durationMonths > MaxDurationMonths {
		return PointsLicensing{}, fmt.Errorf("prazo deve estar entre 1 e %d meses", MaxDurationMonths)
	}
	out := PointsLicensing{MonthlyCosts: make([]float64, durationMonths)}
	if len(licenses) == 0 {
		return out, nil
	}
	if _, err := parseStartDate(startDate); err != nil {
		return PointsLicensing{}, err
	}
	var missing []string
	for i, l := range licenses {
		s, err := EstimateLicense(l, durationMonths, startDate)
		if err != nil {
			name := l.Name
			if name == "" {
				name = fmt.Sprintf("licença %d", i+1)
			}
			missing = append(missing, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		out.Schedules = append(out.Schedules, s)
		for m, row := range s.Months {
			out.MonthlyCosts[m] += row.CostBRL
		}
		out.TotalCostBRL += s.TotalCostBRL
	}
	if len(missing) > 0 {
		return PointsLicensing{}, &IncompleteLicensingError{Items: missing}
	}
	for m := range out.MonthlyCosts {
		out.MonthlyCosts[m] = roundCents(out.MonthlyCosts[m])
	}
	out.TotalCostBRL = roundCents(out.TotalCostBRL)
	return out, nil
}
