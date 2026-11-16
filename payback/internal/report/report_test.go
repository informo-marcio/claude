package report

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/informo-marcio/claude/payback/internal/finance"
	"github.com/informo-marcio/claude/payback/internal/store"
)

func sample(t *testing.T, monthly float64, months int) Input {
	t.Helper()
	policy := finance.CurrentPolicy()
	origin := "ES"
	rate := 5.45
	start := "2026-12-01"
	deal := int64(7731)
	items := []store.Item{
		{Name: "SonicWall TZ280", Kind: "hardware", Quantity: 4, UnitPrice: 980, Currency: "USD", OriginState: &origin, Imported: true},
		{Name: "Instalação e racks", Kind: "software", Quantity: 1, UnitPrice: 6500, Currency: "BRL"},
		{Name: "FlexSpend TZ280", Kind: "points_license", Quantity: 4, License: &store.License{ModelName: "TZ280", DailyPoints: 0.46, PriceUSD: 0.68}},
	}
	capex, err := finance.ComputeCapex([]finance.Item{
		{Name: "SonicWall TZ280", Kind: finance.KindHardware, Quantity: 4, UnitPrice: 980, Currency: finance.USD, OriginState: "ES", Imported: true},
		{Name: "Instalação e racks", Kind: finance.KindSoftware, Quantity: 1, UnitPrice: 6500, Currency: finance.BRL},
	}, finance.CapexParams{ExchangeRate: rate, DestState: "SP", InternalICMS: finance.DefaultInternalICMS()})
	if err != nil {
		t.Fatal(err)
	}
	daily := 0.46
	lic, err := finance.EstimateProjectLicensing([]finance.PointsLicense{{Name: "FlexSpend", DailyPoints: &daily, PriceUSD: 0.68, ExchangeRate: rate, Quantity: 4}}, months, start)
	if err != nil {
		t.Fatal(err)
	}
	in := finance.Input{Capex: capex.Total, MonthlyRevenue: monthly, DurationMonths: months, LossRate: 0.003, LicensingCosts: lic.MonthlyCosts, Settings: policy}
	sc, err := finance.CalculateScenarios(in)
	if err != nil {
		t.Fatal(err)
	}
	var sens []SensitivityRow
	for _, delta := range []float64{-0.2, -0.1, 0, 0.1, 0.2} {
		v := in
		v.MonthlyRevenue = monthly * (1 + delta)
		r, err := finance.Calculate(v)
		if err != nil {
			t.Fatal(err)
		}
		sens = append(sens, SensitivityRow{Delta: delta, MonthlyRevenue: v.MonthlyRevenue, NPV: r.NPV, PaybackMonth: r.PaybackMonth, Viability: r.Viability})
	}
	return Input{
		Project: store.Project{
			ID: 2, OwnerName: "Marcio Talhacolo", ClientName: "MBATEC", Name: "SonicWall TZ280 em 4 filiais", Status: store.StatusActive,
			DurationMonths: months, MonthlyRevenue: monthly, ExpectedLossRate: 0.003, ExchangeRate: &rate, DestState: "SP",
			ContractStartDate: &start, DealID: &deal,
		},
		Items: items, Capex: capex, Licensing: lic, Scenarios: sc,
		Health:            finance.ContractHealth(months, sc.Pessimistic.PaybackMonth),
		IRR:               finance.AssessIRR(sc.Pessimistic.MonthlyIRR, policy.IRRAlertThreshold),
		Sensitivity:       sens,
		Policy:            policy,
		SettingsVersionID: 3,
		GeneratedBy:       "Marcio Talhacolo",
		GeneratedAt:       time.Date(2026, 11, 16, 13, 5, 0, 0, time.UTC),
	}
}

// pdfText extrai o texto com o pdftotext, quando disponível.
func pdfText(t *testing.T, pdf []byte) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext não instalado")
	}
	cmd := exec.Command("pdftotext", "-layout", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestProjectPDF(t *testing.T) {
	cases := map[string]struct {
		monthly float64
		months  int
		verdict string
	}{
		"viavel":   {3100, 36, "Viável"},
		"excecao":  {2500, 36, "Exceção comercial"},
		"inviavel": {1500, 36, "Inviável"},
		"longo":    {2400, 60, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := sample(t, c.monthly, c.months)
			pdf, err := ProjectPDF(in)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatal("saída não é um PDF")
			}
			if dir := os.Getenv("REPORT_OUT"); dir != "" {
				if err := os.WriteFile(dir+"/"+name+".pdf", pdf, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			text := pdfText(t, pdf)
			s := in.Scenarios.Pessimistic
			want := []string{
				"MBATEC", "SonicWall TZ280 em 4 filiais", "Memória de cálculo", "Uso interno",
				money(s.NPV), money(s.Capex), money(s.Commission.Total), money(in.Capex.Difal),
				"Fluxo de caixa mês a mês", "Premissas e ressalvas", "gerado em 16/11/2026 às 10:05 por Marcio Talhacolo",
				"Página 1 de", "origem ES, importado",
			}
			if c.verdict != "" {
				want = append(want, c.verdict)
			}
			for _, w := range want {
				if !strings.Contains(text, w) {
					t.Errorf("o PDF não contém %q", w)
				}
			}
			if strings.Contains(text, "{paginas}") {
				t.Error("o total de páginas não foi preenchido")
			}
			// Todos os meses do contrato aparecem no fluxo.
			last := s.Months[len(s.Months)-1]
			if !strings.Contains(text, money(last.CumulativeDiscounted)) {
				t.Errorf("faltou o saldo do último mês: %s", money(last.CumulativeDiscounted))
			}
		})
	}
}

func TestFileName(t *testing.T) {
	got := FileName(store.Project{ClientName: "Unimed Jaú", Name: "Servidores / Virtuozzo (36m)"})
	if got != "memoria-de-calculo-unimed-jau-servidores-virtuozzo-36m.pdf" {
		t.Errorf("nome = %s", got)
	}
}

func TestFormatting(t *testing.T) {
	for got, want := range map[string]string{
		money(1234567.891): "R$ 1.234.567,89",
		money(-25665.29):   "-R$ 25.665,29",
		money(0):           "R$ 0,00",
		money(-0.001):      "R$ 0,00",
		pct(0.0378):        "3,78%",
		number(5.45, 4):    "5,4500",
		compact(-36000):    "-R$ 36 mil",
		compact(1250000):   "R$ 1,2 mi",
	} {
		if got != want {
			t.Errorf("formatação: %q, esperado %q", got, want)
		}
	}
}
