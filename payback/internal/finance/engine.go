package finance

import (
	"errors"
	"fmt"
	"math"
)

// Input são os insumos de uma proposta para um cenário.
//
// Convenção de datas: CAPEX, comissão e receita de implantação ocorrem no mês
// zero; mensalidade, impostos, perdas, suporte e licenças por pontos ocorrem
// ao fim de cada mês do contrato.
type Input struct {
	// Capex é o investimento inicial em reais (ver ComputeCapex).
	Capex float64 `json:"capex"`
	// MonthlyRevenue é a mensalidade faturada.
	MonthlyRevenue float64 `json:"monthlyRevenue"`
	// DurationMonths é o prazo do contrato; precisa existir na política.
	DurationMonths int `json:"durationMonths"`
	// ImplementationRevenue é a receita de implantação recebida no mês zero.
	ImplementationRevenue float64 `json:"implementationRevenue"`
	// LossRate é a inadimplência esperada sobre a mensalidade (0.05 = 5%).
	LossRate float64 `json:"expectedLossRate"`
	// SupportCost é o custo operacional de suporte por mês.
	SupportCost float64 `json:"monthlySupportCost"`
	// LicensingCosts é o custo mensal de licenças por pontos; vazio ou com
	// exatamente DurationMonths posições.
	LicensingCosts []float64 `json:"monthlyLicensingCosts"`
	// Settings é a política financeira usada no cálculo.
	Settings Settings `json:"settings"`
}

// Validate confere os insumos. O motor não corrige entradas inválidas.
func (in Input) Validate() error {
	var errs []error
	if err := in.Settings.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("política financeira: %w", err))
	}
	if in.DurationMonths < 1 || in.DurationMonths > MaxDurationMonths {
		errs = append(errs, fmt.Errorf("prazo deve estar entre 1 e %d meses", MaxDurationMonths))
	} else if !in.Settings.hasDuration(in.DurationMonths) {
		errs = append(errs, fmt.Errorf("prazo de %d meses não está na política financeira", in.DurationMonths))
	}
	if !isAmount(in.Capex) {
		errs = append(errs, errors.New("CAPEX deve ser um valor não negativo"))
	}
	if !isAmount(in.MonthlyRevenue) {
		errs = append(errs, errors.New("mensalidade deve ser um valor não negativo"))
	}
	if !isAmount(in.ImplementationRevenue) {
		errs = append(errs, errors.New("receita de implantação deve ser um valor não negativo"))
	}
	if !isRate(in.LossRate) {
		errs = append(errs, errors.New("inadimplência esperada deve estar entre 0 e 1"))
	}
	if !isAmount(in.SupportCost) {
		errs = append(errs, errors.New("custo de suporte deve ser um valor não negativo"))
	}
	if n := len(in.LicensingCosts); n != 0 && n != in.DurationMonths {
		errs = append(errs, fmt.Errorf("custos de licença por pontos têm %d meses; o contrato tem %d", n, in.DurationMonths))
	}
	for i, c := range in.LicensingCosts {
		if !isAmount(c) {
			errs = append(errs, fmt.Errorf("custo de licença do mês %d inválido", i+1))
		}
	}
	return errors.Join(errs...)
}

// MonthRow é uma linha do fluxo de caixa mensal.
type MonthRow struct {
	Month            int     `json:"month"`
	GrossRevenue     float64 `json:"grossRevenue"`
	ExpectedLoss     float64 `json:"expectedLoss"`
	CollectedRevenue float64 `json:"collectedRevenue"`
	Taxes            float64 `json:"taxes"`
	SupportCost      float64 `json:"supportCost"`
	LicensingCost    float64 `json:"licensingCost"`
	NetCashFlow      float64 `json:"netCashFlow"`
	DiscountedFlow   float64 `json:"discountedFlow"`
	// CumulativeNominal é o saldo acumulado sem desconto, já com o mês zero.
	CumulativeNominal float64 `json:"cumulativeNominal"`
	// CumulativeDiscounted é o VPL acumulado até o mês.
	CumulativeDiscounted float64 `json:"cumulativeDiscounted"`
}

// Viability é a decisão financeira de uma proposta.
type Viability string

const (
	// Viable cria valor acima da taxa de oportunidade e recupera o caixa
	// dentro da política comercial.
	Viable Viability = "VIAVEL"
	// CommercialException cria valor, mas recupera o caixa depois do prazo
	// da política.
	CommercialException Viability = "EXCECAO_COMERCIAL"
	// NotViable não supera a taxa de oportunidade.
	NotViable Viability = "INVIAVEL"
)

// Analysis é o resultado financeiro de um cenário.
type Analysis struct {
	Capex float64 `json:"capex"`
	// InitialFlow é o fluxo do mês zero: −CAPEX − comissão + implantação líquida.
	InitialFlow float64 `json:"initialFlow"`

	TotalRevenue          float64    `json:"totalRevenue"`
	Taxes                 float64    `json:"taxes"`
	RevenueAfterTaxes     float64    `json:"revenueAfterTaxes"`
	ImplementationRevenue float64    `json:"implementationRevenue"`
	ImplementationTaxes   float64    `json:"implementationTaxes"`
	ImplementationNet     float64    `json:"implementationNet"`
	TotalExpectedLoss     float64    `json:"totalExpectedLoss"`
	TotalSupportCost      float64    `json:"totalSupportCost"`
	TotalLicensingCost    float64    `json:"totalLicensingCost"`
	Commission            Commission `json:"commission"`
	// NetCashAfterRisks é o caixa nominal gerado pelo contrato, depois de
	// impostos, perdas, suporte, licenças e comissão, antes do CAPEX.
	NetCashAfterRisks float64 `json:"netCashAfterRisks"`
	// NetCashResult é NetCashAfterRisks menos o CAPEX.
	NetCashResult float64 `json:"netCashResult"`
	// CashAfterPayback é a soma dos fluxos dos meses posteriores ao payback.
	CashAfterPayback float64 `json:"cashAfterPayback"`

	// NPV é o valor presente líquido à taxa de oportunidade.
	NPV float64 `json:"npv"`
	// ROIOnCapex é NPV / CAPEX, como fração.
	ROIOnCapex float64 `json:"roiOnCapex"`
	// NPVMargin é NPV / NetCashAfterRisks, como fração.
	NPVMargin float64 `json:"npvMargin"`
	// MonthlyIRR é a TIR mensal como fração; nil quando não é calculável.
	MonthlyIRR *float64 `json:"monthlyIrr"`

	// SimplePaybackMonth é o primeiro mês com saldo nominal não negativo.
	SimplePaybackMonth *int `json:"simplePaybackMonth"`
	// FirstCrossingMonth é o primeiro mês em que o saldo descontado fica não
	// negativo, mesmo que volte a ficar negativo depois.
	FirstCrossingMonth *int `json:"firstCrossingMonth"`
	// PaybackMonth é o payback descontado sustentado: o mês a partir do qual
	// o saldo descontado não volta a ficar negativo até o fim do contrato.
	// É o payback usado na decisão, na política e na saúde do contrato.
	PaybackMonth *int `json:"paybackMonth"`
	// MaxPaybackMonth é o limite da política para o prazo.
	MaxPaybackMonth int `json:"maxPaybackMonth"`

	// MinimumEconomicMonthly é a menor mensalidade com VPL não negativo;
	// nil quando nenhuma mensalidade resolve.
	MinimumEconomicMonthly *float64 `json:"minimumEconomicMonthly"`
	// MinimumPolicyMonthly é a menor mensalidade que cumpre o payback da
	// política; nil quando nenhuma mensalidade resolve.
	MinimumPolicyMonthly *float64 `json:"minimumPolicyMonthly"`

	MeetsPaybackPolicy bool       `json:"meetsPaybackPolicy"`
	Viability          Viability  `json:"viability"`
	Months             []MonthRow `json:"months"`
}

// flows é o fluxo de caixa bruto, sem arredondamento, de uma mensalidade.
type flows struct {
	commission  Commission
	implTaxes   float64
	implNet     float64
	initial     float64
	monthlyNet  []float64 // fluxo líquido de cada mês
	discounted  []float64 // saldo descontado acumulado ao fim de cada mês
	nominal     []float64 // saldo nominal acumulado ao fim de cada mês
	npv         float64
	firstCross  *int
	simple      *int
	sustained   *int
	maxPayback  int
	licensing   []float64
	totalLicens float64
}

func intPtr(v int) *int { return &v }

func buildFlows(in Input, monthly float64) flows {
	n := in.DurationMonths
	s := in.Settings
	lic := make([]float64, n)
	copy(lic, in.LicensingCosts)
	totalLic := 0.0
	for _, c := range lic {
		totalLic += c
	}
	f := flows{licensing: lic, totalLicens: totalLic, maxPayback: s.MaxPayback[n]}
	f.commission = computeCommission(monthly, n, in.Capex, totalLic, s.TaxRate, s.Commission[n])
	f.implTaxes = in.ImplementationRevenue * s.TaxRate
	f.implNet = in.ImplementationRevenue - f.implTaxes
	f.initial = -in.Capex - f.commission.Total + f.implNet

	// Impostos incidem sobre o faturado, antes da perda esperada.
	base := monthly - monthly*in.LossRate - monthly*s.TaxRate - in.SupportCost
	nominal, discounted := f.initial, f.initial
	if nominal >= 0 {
		f.simple = intPtr(0)
		f.firstCross = intPtr(0)
	}
	lastNegative := -1
	if discounted < 0 {
		lastNegative = 0
	}
	f.monthlyNet = make([]float64, n)
	f.nominal = make([]float64, n)
	f.discounted = make([]float64, n)
	for m := 1; m <= n; m++ {
		net := base - lic[m-1]
		nominal += net
		discounted += net / math.Pow(1+s.OpportunityRate, float64(m))
		f.monthlyNet[m-1] = net
		f.nominal[m-1] = nominal
		f.discounted[m-1] = discounted
		if f.simple == nil && nominal >= 0 {
			f.simple = intPtr(m)
		}
		if f.firstCross == nil && discounted >= 0 {
			f.firstCross = intPtr(m)
		}
		if discounted < 0 {
			lastNegative = m
		}
	}
	f.npv = discounted
	if lastNegative < n {
		f.sustained = intPtr(lastNegative + 1)
	}
	return f
}

func (f flows) meetsPolicy() bool {
	return f.sustained != nil && *f.sustained <= f.maxPayback
}

func classify(f flows) Viability {
	if f.npv <= 0 || f.sustained == nil {
		return NotViable
	}
	if !f.meetsPolicy() {
		return CommercialException
	}
	return Viable
}

// maxMonthlyCents é o teto da busca da mensalidade mínima (R$ 10 bilhões).
const maxMonthlyCents = int64(1e12)

// solveMinimumMonthly busca, em centavos, a menor mensalidade que satisfaz ok.
// Devolve nil quando nem o teto satisfaz. A busca roda o próprio motor em cada
// candidato, então o piso devolvido é sempre verificado, inclusive nos ramos
// em que a comissão é zero ou a implantação cobre o CAPEX.
func solveMinimumMonthly(in Input, ok func(flows) bool) *float64 {
	test := func(cents int64) bool { return ok(buildFlows(in, float64(cents)/100)) }
	if test(0) {
		zero := 0.0
		return &zero
	}
	hi := int64(100)
	for !test(hi) {
		if hi >= maxMonthlyCents {
			return nil
		}
		hi *= 2
	}
	lo := hi / 2
	if hi == 100 {
		lo = 0
	}
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if test(mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	v := float64(hi) / 100
	return &v
}

// Calculate roda o motor para um cenário.
func Calculate(in Input) (Analysis, error) {
	if err := in.Validate(); err != nil {
		return Analysis{}, err
	}
	n := in.DurationMonths
	s := in.Settings
	m := in.MonthlyRevenue
	f := buildFlows(in, m)

	totalRevenue := in.ImplementationRevenue + m*float64(n)
	taxes := f.implTaxes + m*s.TaxRate*float64(n)
	totalLoss := m * in.LossRate * float64(n)
	totalSupport := in.SupportCost * float64(n)
	netAfterRisks := totalRevenue - taxes - totalLoss - totalSupport - f.totalLicens - f.commission.Total

	a := Analysis{
		Capex:                 roundCents(in.Capex),
		InitialFlow:           roundCents(f.initial),
		TotalRevenue:          roundCents(totalRevenue),
		Taxes:                 roundCents(taxes),
		RevenueAfterTaxes:     roundCents(totalRevenue - taxes),
		ImplementationRevenue: roundCents(in.ImplementationRevenue),
		ImplementationTaxes:   roundCents(f.implTaxes),
		ImplementationNet:     roundCents(f.implNet),
		TotalExpectedLoss:     roundCents(totalLoss),
		TotalSupportCost:      roundCents(totalSupport),
		TotalLicensingCost:    roundCents(f.totalLicens),
		Commission:            f.commission,
		NetCashAfterRisks:     roundCents(netAfterRisks),
		NetCashResult:         roundCents(netAfterRisks - in.Capex),
		NPV:                   roundCents(f.npv),
		SimplePaybackMonth:    f.simple,
		FirstCrossingMonth:    f.firstCross,
		PaybackMonth:          f.sustained,
		MaxPaybackMonth:       f.maxPayback,
		MeetsPaybackPolicy:    f.meetsPolicy(),
		Viability:             classify(f),
		Months:                make([]MonthRow, n),
	}
	if in.Capex > 0 {
		a.ROIOnCapex = roundRate(f.npv / in.Capex)
	}
	if netAfterRisks > 0 {
		a.NPVMargin = roundRate(f.npv / netAfterRisks)
	}
	if irr := monthlyIRR(append([]float64{f.initial}, f.monthlyNet...)); irr != nil {
		v := roundRate(*irr)
		a.MonthlyIRR = &v
	}
	afterPayback := 0.0
	for i := 0; i < n; i++ {
		month := i + 1
		if f.sustained != nil && month > *f.sustained {
			afterPayback += f.monthlyNet[i]
		}
		a.Months[i] = MonthRow{
			Month:                month,
			GrossRevenue:         roundCents(m),
			ExpectedLoss:         roundCents(m * in.LossRate),
			CollectedRevenue:     roundCents(m - m*in.LossRate),
			Taxes:                roundCents(m * s.TaxRate),
			SupportCost:          roundCents(in.SupportCost),
			LicensingCost:        roundCents(f.licensing[i]),
			NetCashFlow:          roundCents(f.monthlyNet[i]),
			DiscountedFlow:       roundCents(f.monthlyNet[i] / math.Pow(1+s.OpportunityRate, float64(month))),
			CumulativeNominal:    roundCents(f.nominal[i]),
			CumulativeDiscounted: roundCents(f.discounted[i]),
		}
	}
	a.CashAfterPayback = roundCents(afterPayback)
	a.MinimumEconomicMonthly = solveMinimumMonthly(in, func(c flows) bool { return c.npv >= 0 })
	a.MinimumPolicyMonthly = solveMinimumMonthly(in, func(c flows) bool { return c.meetsPolicy() })
	return a, nil
}

// Scenarios compara o cenário base (sem inadimplência nem suporte) com o
// pessimista (com as premissas de risco da proposta).
type Scenarios struct {
	Base        Analysis `json:"base"`
	Pessimistic Analysis `json:"pessimistic"`
}

// CalculateScenarios roda os dois cenários. A decisão exibida pelo portal é
// sempre a do pessimista.
func CalculateScenarios(in Input) (Scenarios, error) {
	pessimistic, err := Calculate(in)
	if err != nil {
		return Scenarios{}, err
	}
	base := in
	base.LossRate = 0
	base.SupportCost = 0
	b, err := Calculate(base)
	if err != nil {
		return Scenarios{}, err
	}
	return Scenarios{Base: b, Pessimistic: pessimistic}, nil
}

// monthlyIRR calcula a TIR mensal por bisseção. Devolve nil quando o fluxo
// não tem troca de sinal ou não há raiz no intervalo pesquisado.
func monthlyIRR(cash []float64) *float64 {
	hasNeg, hasPos := false, false
	for _, c := range cash {
		if c < 0 {
			hasNeg = true
		}
		if c > 0 {
			hasPos = true
		}
	}
	if len(cash) < 2 || !hasNeg || !hasPos {
		return nil
	}
	npvAt := func(rate float64) float64 {
		sum := 0.0
		for t, c := range cash {
			sum += c / math.Pow(1+rate, float64(t))
		}
		return sum
	}
	lower, upper := -0.9999, 1.0
	lowerNPV, upperNPV := npvAt(lower), npvAt(upper)
	for lowerNPV*upperNPV > 0 && upper < 1024 {
		upper *= 2
		upperNPV = npvAt(upper)
	}
	if lowerNPV*upperNPV > 0 || math.IsNaN(lowerNPV*upperNPV) {
		return nil
	}
	for i := 0; i < 200; i++ {
		mid := (lower + upper) / 2
		midNPV := npvAt(mid)
		if math.Abs(midNPV) < 1e-7 {
			return &mid
		}
		if lowerNPV*midNPV > 0 {
			lower, lowerNPV = mid, midNPV
		} else {
			upper = mid
		}
	}
	mid := (lower + upper) / 2
	return &mid
}
