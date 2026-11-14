package finance

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// Os exemplos em testdata/exemplos_manus.json foram gerados pelo motor do
// portal anterior (checkpoint ce0935ea) com dados sintéticos. Este teste prova
// que, nas regras que não mudaram, o motor em Go produz os mesmos números.

type legacyMonth struct {
	Month                 int     `json:"month"`
	GrossRevenue          float64 `json:"grossRevenue"`
	ExpectedCreditLoss    float64 `json:"expectedCreditLoss"`
	CollectedRevenue      float64 `json:"collectedRevenue"`
	Taxes                 float64 `json:"taxes"`
	SupportCost           float64 `json:"supportCost"`
	LicensingCost         float64 `json:"licensingCost"`
	NetCashFlow           float64 `json:"netCashFlow"`
	DiscountedNetCashFlow float64 `json:"discountedNetCashFlow"`
	CumulativeRevenue     float64 `json:"cumulativeRevenue"`
	Balance               float64 `json:"balance"`
}

type legacyResult struct {
	TotalCost                 float64 `json:"totalCost"`
	InitialNetInvestment      float64 `json:"initialNetInvestment"`
	TotalProjectedRevenue     float64 `json:"totalProjectedRevenue"`
	Taxes                     float64 `json:"taxes"`
	RevenueAfterTaxes         float64 `json:"revenueAfterTaxes"`
	ImplementationNetCashFlow float64 `json:"implementationNetCashFlow"`
	TotalExpectedCreditLoss   float64 `json:"totalExpectedCreditLoss"`
	TotalSupportCost          float64 `json:"totalSupportCost"`
	TotalLicensingCost        float64 `json:"totalLicensingCost"`
	Commission                struct {
		GrossCommissionBase   float64 `json:"grossCommissionBase"`
		CommissionTaxes       float64 `json:"commissionTaxes"`
		TaxableCommissionBase float64 `json:"taxableCommissionBase"`
		SellerCommission      float64 `json:"sellerCommission"`
		ArchitectCommission   float64 `json:"architectCommission"`
		ManagerCommission     float64 `json:"managerCommission"`
		TotalCommission       float64 `json:"totalCommission"`
	} `json:"commission"`
	NetCashAfterRisks         float64       `json:"netCashAfterRisks"`
	NetCashResult             float64       `json:"netCashResult"`
	CashGeneratedAfterPayback float64       `json:"cashGeneratedAfterPayback"`
	NetPresentValue           float64       `json:"netPresentValue"`
	SimplePaybackMonth        *int          `json:"simplePaybackMonth"`
	DiscountedPaybackMonth    *int          `json:"discountedPaybackMonth"`
	ROI                       float64       `json:"roi"`
	NetMargin                 float64       `json:"netMargin"`
	MonthlyIRR                *float64      `json:"monthlyIrr"`
	MinimumEconomicMonthly    float64       `json:"minimumEconomicMonthly"`
	MinimumPolicyMonthly      float64       `json:"minimumPolicyMonthly"`
	MaxPaybackMonth           int           `json:"maxPaybackMonth"`
	MonthlyData               []legacyMonth `json:"monthlyData"`
	MeetsPaybackPolicy        bool          `json:"meetsPaybackPolicy"`
	ViabilityStatus           string        `json:"viabilityStatus"`
}

type legacyExample struct {
	Input struct {
		TotalCost             float64   `json:"totalCost"`
		MonthlyRevenue        float64   `json:"monthlyRevenue"`
		DurationMonths        int       `json:"durationMonths"`
		ImplementationRevenue float64   `json:"implementationRevenue"`
		ExpectedLossRate      float64   `json:"expectedLossRate"`
		MonthlySupportCost    float64   `json:"monthlySupportCost"`
		MonthlyLicensingCosts []float64 `json:"monthlyLicensingCosts"`
		FinancialSettings     Settings  `json:"financialSettings"`
	} `json:"input"`
	Result     legacyResult `json:"result"`
	BaseResult legacyResult `json:"baseResult"`
	Health     struct {
		Status string `json:"status"`
		Score  int    `json:"score"`
	} `json:"health"`
	IRRAssessment struct {
		Status string `json:"status"`
	} `json:"irrAssessment"`
	LicensingInput *struct {
		DailyPoints    float64 `json:"dailyPoints"`
		PriceUSD       float64 `json:"priceUsd"`
		ExchangeRate   float64 `json:"exchangeRate"`
		Quantity       int     `json:"quantity"`
		DurationMonths int     `json:"durationMonths"`
		StartDate      string  `json:"startDate"`
	} `json:"licensingInput"`
	LicensingSchedule *struct {
		TotalDays    int     `json:"totalDays"`
		TotalPoints  float64 `json:"totalPoints"`
		TotalCostUSD float64 `json:"totalCostUsd"`
		TotalCostBRL float64 `json:"totalCostBrl"`
		MonthlyData  []struct {
			StartDate        string  `json:"startDate"`
			EndDateExclusive string  `json:"endDateExclusive"`
			Days             int     `json:"days"`
			Points           float64 `json:"points"`
			CostBRL          float64 `json:"costBrl"`
		} `json:"monthlyData"`
	} `json:"licensingSchedule"`
	ProjectLicensing *struct {
		MonthlyCosts []float64 `json:"monthlyCosts"`
	} `json:"projectLicensing"`
}

func loadLegacy(t *testing.T) map[string]legacyExample {
	t.Helper()
	raw, err := os.ReadFile("testdata/exemplos_manus.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		A legacyExample `json:"exampleA"`
		B legacyExample `json:"exampleB"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return map[string]legacyExample{"A": doc.A, "B": doc.B}
}

func (e legacyExample) toInput() Input {
	return Input{
		Capex:                 e.Input.TotalCost,
		MonthlyRevenue:        e.Input.MonthlyRevenue,
		DurationMonths:        e.Input.DurationMonths,
		ImplementationRevenue: e.Input.ImplementationRevenue,
		LossRate:              e.Input.ExpectedLossRate,
		SupportCost:           e.Input.MonthlySupportCost,
		LicensingCosts:        e.Input.MonthlyLicensingCosts,
		Settings:              e.Input.FinancialSettings,
	}
}

func eq(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, esperado %v", name, got, want)
	}
}

func eqMonth(t *testing.T, name string, got, want *int) {
	t.Helper()
	if (got == nil) != (want == nil) || (got != nil && *got != *want) {
		t.Errorf("%s = %v, esperado %v", name, fmtPtr(got), fmtPtr(want))
	}
}

func fmtPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func compareLegacy(t *testing.T, got Analysis, want legacyResult) {
	t.Helper()
	eq(t, "capex", got.Capex, want.TotalCost)
	eq(t, "fluxo inicial", -got.InitialFlow, want.InitialNetInvestment)
	eq(t, "receita total", got.TotalRevenue, want.TotalProjectedRevenue)
	eq(t, "impostos", got.Taxes, want.Taxes)
	eq(t, "receita após impostos", got.RevenueAfterTaxes, want.RevenueAfterTaxes)
	eq(t, "implantação líquida", got.ImplementationNet, want.ImplementationNetCashFlow)
	eq(t, "perda esperada", got.TotalExpectedLoss, want.TotalExpectedCreditLoss)
	eq(t, "suporte", got.TotalSupportCost, want.TotalSupportCost)
	eq(t, "licenças", got.TotalLicensingCost, want.TotalLicensingCost)
	eq(t, "base bruta da comissão", got.Commission.GrossBase, want.Commission.GrossCommissionBase)
	eq(t, "impostos da comissão", got.Commission.Taxes, want.Commission.CommissionTaxes)
	eq(t, "base líquida da comissão", got.Commission.NetBase, want.Commission.TaxableCommissionBase)
	eq(t, "comissão vendedor", got.Commission.Seller, want.Commission.SellerCommission)
	eq(t, "comissão arquiteto", got.Commission.Architect, want.Commission.ArchitectCommission)
	eq(t, "comissão gerente", got.Commission.Manager, want.Commission.ManagerCommission)
	eq(t, "comissão total", got.Commission.Total, want.Commission.TotalCommission)
	eq(t, "caixa após riscos", got.NetCashAfterRisks, want.NetCashAfterRisks)
	eq(t, "resultado líquido de caixa", got.NetCashResult, want.NetCashResult)
	eq(t, "caixa após payback", got.CashAfterPayback, want.CashGeneratedAfterPayback)
	eq(t, "VPL", got.NPV, want.NetPresentValue)
	eq(t, "ROI (%)", roundTo(got.ROIOnCapex*100, 4), want.ROI)
	eq(t, "margem (%)", roundTo(got.NPVMargin*100, 4), want.NetMargin)
	if (got.MonthlyIRR == nil) != (want.MonthlyIRR == nil) {
		t.Errorf("TIR disponível = %v, esperado %v", got.MonthlyIRR != nil, want.MonthlyIRR != nil)
	} else if got.MonthlyIRR != nil {
		eq(t, "TIR (%)", roundTo(*got.MonthlyIRR*100, 4), *want.MonthlyIRR)
	}
	eqMonth(t, "payback simples", got.SimplePaybackMonth, want.SimplePaybackMonth)
	eqMonth(t, "primeiro cruzamento", got.FirstCrossingMonth, want.DiscountedPaybackMonth)
	eqMonth(t, "payback sustentado", got.PaybackMonth, want.DiscountedPaybackMonth)
	if got.MaxPaybackMonth != want.MaxPaybackMonth {
		t.Errorf("payback máximo = %d, esperado %d", got.MaxPaybackMonth, want.MaxPaybackMonth)
	}
	if got.MeetsPaybackPolicy != want.MeetsPaybackPolicy {
		t.Errorf("atende política = %v, esperado %v", got.MeetsPaybackPolicy, want.MeetsPaybackPolicy)
	}
	if string(got.Viability) != want.ViabilityStatus {
		t.Errorf("viabilidade = %s, esperado %s", got.Viability, want.ViabilityStatus)
	}
	if got.MinimumEconomicMonthly == nil || got.MinimumPolicyMonthly == nil {
		t.Fatalf("mensalidades mínimas indisponíveis")
	}
	eq(t, "piso econômico", *got.MinimumEconomicMonthly, want.MinimumEconomicMonthly)
	eq(t, "piso da política", *got.MinimumPolicyMonthly, want.MinimumPolicyMonthly)
	if len(got.Months) != len(want.MonthlyData) {
		t.Fatalf("meses = %d, esperado %d", len(got.Months), len(want.MonthlyData))
	}
	for i, w := range want.MonthlyData {
		g := got.Months[i]
		if g.Month != w.Month {
			t.Errorf("mês %d fora de ordem", w.Month)
		}
		eq(t, "mês: receita bruta", g.GrossRevenue, w.GrossRevenue)
		eq(t, "mês: perda", g.ExpectedLoss, w.ExpectedCreditLoss)
		eq(t, "mês: recebido", g.CollectedRevenue, w.CollectedRevenue)
		eq(t, "mês: impostos", g.Taxes, w.Taxes)
		eq(t, "mês: suporte", g.SupportCost, w.SupportCost)
		eq(t, "mês: licença", g.LicensingCost, w.LicensingCost)
		eq(t, "mês: fluxo líquido", g.NetCashFlow, w.NetCashFlow)
		eq(t, "mês: fluxo descontado", g.DiscountedFlow, w.DiscountedNetCashFlow)
		eq(t, "mês: acumulado nominal", g.CumulativeNominal, w.CumulativeRevenue)
		eq(t, "mês: acumulado descontado", g.CumulativeDiscounted, w.Balance)
	}
}

func TestEquivalenceWithPreviousEngine(t *testing.T) {
	for name, ex := range loadLegacy(t) {
		t.Run("exemplo "+name, func(t *testing.T) {
			sc, err := CalculateScenarios(ex.toInput())
			if err != nil {
				t.Fatal(err)
			}
			t.Run("pessimista", func(t *testing.T) { compareLegacy(t, sc.Pessimistic, ex.Result) })
			t.Run("base", func(t *testing.T) { compareLegacy(t, sc.Base, ex.BaseResult) })

			h := ContractHealth(ex.Input.DurationMonths, sc.Pessimistic.PaybackMonth)
			if string(h.Status) != ex.Health.Status || h.Score != ex.Health.Score {
				t.Errorf("saúde = %s/%d, esperado %s/%d", h.Status, h.Score, ex.Health.Status, ex.Health.Score)
			}
			irr := AssessIRR(sc.Pessimistic.MonthlyIRR, ex.Input.FinancialSettings.IRRAlertThreshold)
			if string(irr.Status) != ex.IRRAssessment.Status {
				t.Errorf("alerta de TIR = %s, esperado %s", irr.Status, ex.IRRAssessment.Status)
			}
		})
	}
}

func TestLicensingEquivalenceWithPreviousEngine(t *testing.T) {
	ex := loadLegacy(t)["B"]
	in, want := ex.LicensingInput, ex.LicensingSchedule
	if in == nil || want == nil {
		t.Fatal("exemplo B sem dados de licenciamento")
	}
	lic := PointsLicense{Name: "sintética", DailyPoints: &in.DailyPoints, PriceUSD: in.PriceUSD, ExchangeRate: in.ExchangeRate, Quantity: in.Quantity}
	got, err := EstimateLicense(lic, in.DurationMonths, in.StartDate)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalDays != want.TotalDays {
		t.Errorf("dias = %d, esperado %d", got.TotalDays, want.TotalDays)
	}
	eq(t, "pontos", got.TotalPoints, want.TotalPoints)
	eq(t, "custo USD", got.TotalCostUSD, want.TotalCostUSD)
	eq(t, "custo BRL", got.TotalCostBRL, want.TotalCostBRL)
	for i, w := range want.MonthlyData {
		g := got.Months[i]
		if g.StartDate != w.StartDate || g.EndDateExclusive != w.EndDateExclusive || g.Days != w.Days {
			t.Errorf("mês %d: %s..%s (%d dias), esperado %s..%s (%d dias)", i+1, g.StartDate, g.EndDateExclusive, g.Days, w.StartDate, w.EndDateExclusive, w.Days)
		}
		eq(t, "mês: pontos", g.Points, w.Points)
		eq(t, "mês: custo BRL", g.CostBRL, w.CostBRL)
	}
	project, err := EstimateProjectLicensing([]PointsLicense{lic}, in.DurationMonths, in.StartDate)
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range ex.ProjectLicensing.MonthlyCosts {
		eq(t, "custo mensal do projeto", project.MonthlyCosts[i], w)
	}
}
