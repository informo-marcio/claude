package finance

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// zeroPolicy é uma política sem impostos, desconto ou comissão, para testes
// em que a conta pode ser feita de cabeça.
func zeroPolicy(duration, maxPayback int) Settings {
	return Settings{
		Durations:  []int{duration},
		MaxPayback: map[int]int{duration: maxPayback},
		Commission: map[int]CommissionRates{duration: {}},
	}
}

func mustCalculate(t *testing.T, in Input) Analysis {
	t.Helper()
	a, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCurrentPolicyIsValid(t *testing.T) {
	if err := CurrentPolicy().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSimpleContractWithoutRates(t *testing.T) {
	a := mustCalculate(t, Input{Capex: 100, MonthlyRevenue: 50, DurationMonths: 3, Settings: zeroPolicy(3, 3)})
	eq(t, "VPL", a.NPV, 50)
	eqMonth(t, "payback", a.PaybackMonth, intPtr(2))
	if a.Viability != Viable {
		t.Errorf("viabilidade = %s", a.Viability)
	}
	eq(t, "piso econômico", *a.MinimumEconomicMonthly, 33.34)
	eq(t, "caixa após payback", a.CashAfterPayback, 50)
}

// No portal anterior, uma implantação que cobria o CAPEX registrava payback no
// mês zero mesmo quando o suporte consumia o caixa depois (VPL −250).
func TestPaybackMustBeSustained(t *testing.T) {
	a := mustCalculate(t, Input{
		Capex: 100, ImplementationRevenue: 150, SupportCost: 100,
		DurationMonths: 3, Settings: zeroPolicy(3, 3),
	})
	eq(t, "VPL", a.NPV, -250)
	eqMonth(t, "primeiro cruzamento", a.FirstCrossingMonth, intPtr(0))
	eqMonth(t, "payback sustentado", a.PaybackMonth, nil)
	if a.Viability != NotViable || a.MeetsPaybackPolicy {
		t.Errorf("viabilidade = %s, atende política = %v", a.Viability, a.MeetsPaybackPolicy)
	}
	if h := ContractHealth(3, a.PaybackMonth); h.Status != HealthCritical || h.HasPayback {
		t.Errorf("saúde = %+v", h)
	}
	eq(t, "caixa após payback", a.CashAfterPayback, 0)
}

// Rodar o motor no piso devolvido precisa sempre confirmar o piso.
func assertFloorsHold(t *testing.T, in Input) Analysis {
	t.Helper()
	a := mustCalculate(t, in)
	if a.MinimumEconomicMonthly != nil {
		at := in
		at.MonthlyRevenue = *a.MinimumEconomicMonthly
		if r := buildFlows(at, at.MonthlyRevenue); r.npv < 0 {
			t.Errorf("no piso econômico %.2f o VPL é %.4f", at.MonthlyRevenue, r.npv)
		}
		if at.MonthlyRevenue >= 0.01 {
			if r := buildFlows(at, at.MonthlyRevenue-0.01); r.npv >= 0 {
				t.Errorf("piso econômico %.2f não é o menor valor", at.MonthlyRevenue)
			}
		}
	}
	if a.MinimumPolicyMonthly != nil {
		at := in
		at.MonthlyRevenue = *a.MinimumPolicyMonthly
		if r := mustCalculate(t, at); !r.MeetsPaybackPolicy {
			t.Errorf("no piso da política %.2f o payback é %v (máx. %d)", at.MonthlyRevenue, fmtPtr(r.PaybackMonth), r.MaxPaybackMonth)
		}
	}
	return a
}

func legacyDefaults() Settings {
	s := CurrentPolicy()
	s.TaxRate, s.OpportunityRate = 0.22, 0.01
	s.MaxPayback = map[int]int{12: 11, 24: 17, 36: 25, 48: 34, 60: 42}
	return s
}

// Caso documentado pelo Manus: o piso de R$ 15,10 da fórmula antiga deixava o
// VPL em −87,44 porque a fórmula supunha comissão positiva.
func TestMinimumMonthlyWhenCommissionIsZero(t *testing.T) {
	in := Input{Capex: 1000, ImplementationRevenue: 1000, DurationMonths: 12, Settings: legacyDefaults()}
	a := assertFloorsHold(t, in)
	if a.MinimumEconomicMonthly == nil || *a.MinimumEconomicMonthly <= 15.10 {
		t.Fatalf("piso econômico = %v, deveria ser maior que o valor antigo de 15,10", a.MinimumEconomicMonthly)
	}
}

// Caso documentado pelo Manus: a implantação cobre o CAPEX, o piso antigo era
// zero, mas o suporte de R$ 100/mês deixava o VPL em −1.069,51.
func TestMinimumMonthlyWhenImplementationCoversCapex(t *testing.T) {
	in := Input{Capex: 100, ImplementationRevenue: 200, SupportCost: 100, DurationMonths: 12, Settings: legacyDefaults()}
	a := assertFloorsHold(t, in)
	eq(t, "VPL sem mensalidade", a.NPV, -1069.51)
	if a.MinimumEconomicMonthly == nil || *a.MinimumEconomicMonthly <= 0 {
		t.Fatalf("piso econômico = %v, deveria ser positivo", a.MinimumEconomicMonthly)
	}
}

func TestMinimumMonthlyHasNoSolution(t *testing.T) {
	in := Input{Capex: 100, MonthlyRevenue: 1, LossRate: 1, DurationMonths: 12, Settings: legacyDefaults()}
	a := mustCalculate(t, in)
	if a.MinimumEconomicMonthly != nil || a.MinimumPolicyMonthly != nil {
		t.Errorf("com 100%% de perda não há mensalidade que resolva: %v / %v", a.MinimumEconomicMonthly, a.MinimumPolicyMonthly)
	}
	if a.Viability != NotViable {
		t.Errorf("viabilidade = %s", a.Viability)
	}
}

func TestMinimumMonthlyAcrossCurrentPolicy(t *testing.T) {
	s := CurrentPolicy()
	lic := make([]float64, 36)
	for i := range lic {
		lic[i] = 40 + float64(i%3)
	}
	cases := []Input{
		{Capex: 34000, MonthlyRevenue: 2500, ImplementationRevenue: 3000, DurationMonths: 36},
		{Capex: 50000, MonthlyRevenue: 1, DurationMonths: 12, LossRate: 0.02, SupportCost: 150},
		{Capex: 9000, MonthlyRevenue: 900, DurationMonths: 36, LicensingCosts: lic, LossRate: s.DefaultLossRate},
		{Capex: 0, MonthlyRevenue: 0, DurationMonths: 24, SupportCost: 80},
		{Capex: 250000, MonthlyRevenue: 9000, ImplementationRevenue: 20000, DurationMonths: 60},
	}
	for _, in := range cases {
		in.Settings = s
		assertFloorsHold(t, in)
	}
}

// Projeto real conferido no portal anterior em 14/11/2026 (FORTIGATE 90G):
// as regras desses indicadores não mudaram, então os números devem coincidir.
func TestMatchesProjectSeenInPreviousPortal(t *testing.T) {
	a := mustCalculate(t, Input{
		Capex: 34000, MonthlyRevenue: 2500, ImplementationRevenue: 3000,
		DurationMonths: 36, Settings: CurrentPolicy(),
	})
	eq(t, "comissão", a.Commission.Total, 4050.14)
	eq(t, "impostos", a.Taxes, 24645)
	eq(t, "caixa após riscos", a.NetCashAfterRisks, 64304.86)
	eq(t, "resultado líquido", a.NetCashResult, 30304.86)
	eq(t, "VPL", a.NPV, 18534.27)
	eq(t, "caixa após payback", a.CashAfterPayback, 23887.50)
	eqMonth(t, "payback", a.PaybackMonth, intPtr(23))
	eq(t, "TIR (%)", roundTo(*a.MonthlyIRR*100, 2), 3.78)
	eq(t, "ROI (%)", roundTo(a.ROIOnCapex*100, 2), 54.51)
	eq(t, "piso econômico", *a.MinimumEconomicMonthly, 1532.06)
	eq(t, "piso da política", *a.MinimumPolicyMonthly, 2287.25)
	if a.Viability != Viable {
		t.Errorf("viabilidade = %s", a.Viability)
	}
	if h := ContractHealth(36, a.PaybackMonth); h.Status != HealthAdequate || h.Score != 36 || h.BufferMonths != 13 {
		t.Errorf("saúde = %+v", h)
	}
}

func TestViabilityClasses(t *testing.T) {
	s := CurrentPolicy()
	base := Input{Capex: 34000, ImplementationRevenue: 3000, DurationMonths: 36, Settings: s}
	for monthly, want := range map[float64]Viability{2500: Viable, 2000: CommercialException, 1200: NotViable} {
		in := base
		in.MonthlyRevenue = monthly
		if a := mustCalculate(t, in); a.Viability != want {
			t.Errorf("mensalidade %.0f: %s, esperado %s (payback %v)", monthly, a.Viability, want, fmtPtr(a.PaybackMonth))
		}
	}
}

func TestCommissionIsZeroOnNegativeMargin(t *testing.T) {
	c := computeCommission(100, 12, 5000, 0, 0.265, CurrentPolicy().Commission[12])
	if c.Total != 0 || c.GrossBase != 0 {
		t.Errorf("comissão sobre margem negativa: %+v", c)
	}
}

func TestIRRUnavailableWithoutSignChange(t *testing.T) {
	a := mustCalculate(t, Input{Capex: 0, MonthlyRevenue: 0, DurationMonths: 12, Settings: CurrentPolicy()})
	if a.MonthlyIRR != nil {
		t.Errorf("TIR = %v, esperado indisponível", *a.MonthlyIRR)
	}
	if got := AssessIRR(a.MonthlyIRR, 0.016); got.Status != IRRUnavailable || got.Difference != nil {
		t.Errorf("avaliação = %+v", got)
	}
	exact := 0.016
	if got := AssessIRR(&exact, 0.016); got.Status != IRRAboveThreshold {
		t.Errorf("TIR igual ao limite deve atender: %+v", got)
	}
}

func TestHealthBands(t *testing.T) {
	for payback, want := range map[int]HealthStatus{4: HealthExcellent, 6: HealthGood, 8: HealthAdequate, 10: HealthLimit, 11: HealthCritical, 12: HealthCritical} {
		if h := ContractHealth(12, intPtr(payback)); h.Status != want {
			t.Errorf("payback %d de 12: %s, esperado %s", payback, h.Status, want)
		}
	}
}

func TestInvalidInputIsRejected(t *testing.T) {
	ok := Input{Capex: 1000, MonthlyRevenue: 100, DurationMonths: 12, Settings: CurrentPolicy()}
	cases := map[string]func(*Input){
		"prazo fora da política":  func(in *Input) { in.DurationMonths = 18 },
		"prazo zero":              func(in *Input) { in.DurationMonths = 0 },
		"CAPEX negativo":          func(in *Input) { in.Capex = -1 },
		"mensalidade NaN":         func(in *Input) { in.MonthlyRevenue = math.NaN() },
		"perda acima de 100%":     func(in *Input) { in.LossRate = 1.2 },
		"licenças de outro prazo": func(in *Input) { in.LicensingCosts = make([]float64, 5) },
		"política inválida":       func(in *Input) { in.Settings.TaxRate = 2 },
		"política sem payback":    func(in *Input) { in.Settings.MaxPayback = map[int]int{} },
	}
	if _, err := Calculate(ok); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range cases {
		in := ok
		in.Settings = CurrentPolicy()
		mutate(&in)
		if _, err := Calculate(in); err == nil {
			t.Errorf("%s: esperado erro", name)
		}
	}
}

func TestDifal(t *testing.T) {
	icms := DefaultInternalICMS()
	cases := []struct {
		name         string
		origin, dest string
		imported     bool
		interstate   float64
		rate         float64
	}{
		{"Paraná para São Paulo", "PR", "SP", false, 0.12, 0.06},
		{"Espírito Santo para São Paulo", "ES", "SP", false, 0.12, 0.06},
		{"Bahia para São Paulo", "BA", "SP", false, 0.12, 0.06},
		{"importado para São Paulo", "ES", "SP", true, 0.04, 0.14},
		{"São Paulo para Bahia", "SP", "BA", false, 0.07, 0.135},
		{"São Paulo para Espírito Santo", "SP", "ES", false, 0.07, 0.10},
		{"São Paulo para Minas Gerais", "sp", " mg ", false, 0.12, 0.06},
		{"mesma UF", "SP", "SP", true, 0, 0},
	}
	for _, c := range cases {
		got, err := Difal(10000, c.origin, c.dest, c.imported, icms)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if math.Abs(got.InterstateRate-c.interstate) > 1e-9 || math.Abs(got.Rate-c.rate) > 1e-9 || math.Abs(got.Value-10000*c.rate) > 1e-9 {
			t.Errorf("%s: %+v, esperado interestadual %v e DIFAL %v", c.name, got, c.interstate, c.rate)
		}
	}
	if _, err := Difal(100, "XX", "SP", false, icms); err == nil {
		t.Error("UF desconhecida deve gerar erro, não DIFAL zero")
	}
}

func TestCapexFollowsExchangeRate(t *testing.T) {
	items := []Item{
		{Name: "Firewall", Kind: KindHardware, Quantity: 2, UnitPrice: 500, Currency: USD, OriginState: "ES", Imported: true},
		{Name: "Licença", Kind: KindSoftware, Quantity: 1, UnitPrice: 1200, Currency: BRL},
		{Name: "Pontos", Kind: KindPointsLicense, Quantity: 3},
	}
	p := CapexParams{ExchangeRate: 5, DestState: "SP", InternalICMS: DefaultInternalICMS()}
	c, err := ComputeCapex(items, p)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "hardware", c.Hardware, 5000)
	eq(t, "software", c.Software, 1200)
	eq(t, "DIFAL", c.Difal, 700)
	eq(t, "total", c.Total, 6900)
	if len(c.Items) != 2 {
		t.Errorf("licença por pontos não entra no CAPEX: %d itens", len(c.Items))
	}

	p.ExchangeRate = 6
	c, err = ComputeCapex(items, p)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "hardware com novo câmbio", c.Hardware, 6000)
	eq(t, "DIFAL com novo câmbio", c.Difal, 840)
	eq(t, "total com novo câmbio", c.Total, 8040)

	p.ExchangeRate = 0
	if _, err := ComputeCapex(items, p); err == nil {
		t.Error("item em dólar sem câmbio deve gerar erro")
	}
}

func TestLicensingCalendar(t *testing.T) {
	daily := 1.0
	lic := PointsLicense{Name: "FortiGate 40F", DailyPoints: &daily, PriceUSD: 1, ExchangeRate: 5, Quantity: 1}
	s, err := EstimateLicense(lic, 3, "2028-01-31")
	if err != nil {
		t.Fatal(err)
	}
	wantDays := []int{29, 31, 30} // 31/01→29/02 (bissexto)→31/03→30/04
	for i, d := range wantDays {
		if s.Months[i].Days != d {
			t.Errorf("mês %d: %d dias, esperado %d", i+1, s.Months[i].Days, d)
		}
	}
	eq(t, "custo do primeiro mês", s.Months[0].CostBRL, 145)

	for _, bad := range []string{"2027-02-30", "31/01/2028", ""} {
		if _, err := EstimateLicense(lic, 3, bad); err == nil {
			t.Errorf("data %q deve ser rejeitada", bad)
		}
	}
}

func TestPendingLicenseBlocksTheProject(t *testing.T) {
	daily := 0.92
	ready := PointsLicense{Name: "FortiGate 40F", DailyPoints: &daily, PriceUSD: 0.96, ExchangeRate: 5, Quantity: 2}
	pending := PointsLicense{Name: "FortiGate 90G", PriceUSD: 0.96, ExchangeRate: 5, Quantity: 1}

	if _, err := EstimateLicense(pending, 12, "2026-11-14"); !errors.Is(err, ErrPendingConsumption) {
		t.Errorf("erro = %v, esperado consumo pendente", err)
	}
	_, err := EstimateProjectLicensing([]PointsLicense{ready, pending}, 12, "2026-11-14")
	var incomplete *IncompleteLicensingError
	if !errors.As(err, &incomplete) || len(incomplete.Items) != 1 || !strings.Contains(incomplete.Items[0], "FortiGate 90G") {
		t.Fatalf("erro = %v, esperado pendência da FortiGate 90G", err)
	}

	p, err := EstimateProjectLicensing([]PointsLicense{ready}, 12, "2026-11-14")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.MonthlyCosts) != 12 || p.TotalCostBRL <= 0 {
		t.Errorf("custos = %+v", p)
	}
	none, err := EstimateProjectLicensing(nil, 12, "")
	if err != nil || len(none.MonthlyCosts) != 12 || none.TotalCostBRL != 0 {
		t.Errorf("sem licenças: %+v, %v", none, err)
	}
}
