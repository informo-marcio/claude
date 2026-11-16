// Package report gera a memória de cálculo de um projeto em PDF.
//
// O documento é montado no servidor a partir do mesmo resultado que a tela
// exibe; nenhum número é recalculado aqui. A identidade segue o Guia de
// Identidade Visual da Informo: logotipo em cores sobre branco, verde
// institucional #009aa3 e tipografia Roboto.
package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"strings"
	"time"
	_ "time/tzdata" // o fuso de Brasília precisa existir mesmo em imagem mínima

	"github.com/go-pdf/fpdf"

	"github.com/informo-marcio/claude/payback/internal/finance"
	"github.com/informo-marcio/claude/payback/internal/store"
)

//go:embed assets/logo-informo.png
var logoPNG []byte

//go:embed assets/Roboto-Light.ttf
var robotoLight []byte

//go:embed assets/Roboto-Regular.ttf
var robotoRegular []byte

//go:embed assets/Roboto-Medium.ttf
var robotoMedium []byte

//go:embed assets/Roboto-Bold.ttf
var robotoBold []byte

// SensitivityRow é uma linha da tabela de sensibilidade à mensalidade.
type SensitivityRow struct {
	Delta          float64
	MonthlyRevenue float64
	NPV            float64
	PaybackMonth   *int
	Viability      finance.Viability
}

// Input reúne tudo que o relatório mostra, já calculado.
type Input struct {
	Project           store.Project
	Items             []store.Item
	Capex             finance.Capex
	Licensing         finance.PointsLicensing
	Scenarios         finance.Scenarios
	Health            finance.Health
	IRR               finance.IRRAssessment
	Sensitivity       []SensitivityRow
	Policy            finance.Settings
	SettingsVersionID int64
	GeneratedBy       string
	GeneratedAt       time.Time
}

type rgb struct{ r, g, b int }

var (
	brand = rgb{0, 154, 163}
	ink   = rgb{17, 21, 23}
	muted = rgb{88, 99, 103}
	rule  = rgb{214, 220, 221}
	soft  = rgb{244, 246, 246}
	good  = rgb{27, 122, 82}
	warn  = rgb{143, 88, 4}
	amber = rgb{217, 152, 42}
	bad   = rgb{178, 56, 44}
)

const (
	pageW   = 210.0
	pageH   = 297.0
	margin  = 16.0
	contW   = pageW - 2*margin
	bottomY = pageH - 20.0
	// Proporção do logotipo oficial (1510 × 213 px).
	logoRatio = 213.0 / 1510.0
)

type doc struct {
	*fpdf.Fpdf
	in Input
}

// ProjectPDF gera a memória de cálculo do projeto.
func ProjectPDF(in Input) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(margin, margin, margin)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddUTF8FontFromBytes("Roboto", "", robotoRegular)
	pdf.AddUTF8FontFromBytes("Roboto", "B", robotoBold)
	pdf.AddUTF8FontFromBytes("RobotoLight", "", robotoLight)
	pdf.AddUTF8FontFromBytes("RobotoMedium", "", robotoMedium)
	pdf.RegisterImageOptionsReader("logo", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(logoPNG))
	title := fmt.Sprintf("Memória de cálculo: %s, %s", in.Project.ClientName, in.Project.Name)
	pdf.SetTitle(title, true)
	pdf.SetAuthor("Informo", true)
	pdf.SetCreator("Informo Payback Portal", true)

	d := &doc{Fpdf: pdf, in: in}
	pdf.AliasNbPages("{paginas}")
	pdf.SetFooterFunc(d.footer)

	pdf.AddPage()
	d.header()
	d.verdict()
	d.chart()
	d.conditionsAndLedger()
	d.items()
	d.pricing()
	d.monthly()
	d.assumptions()

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("geração do PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// FileName sugere um nome de arquivo sem caracteres problemáticos.
func FileName(p store.Project) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
				b.WriteRune(r)
			case strings.ContainsRune("áàâã", r):
				b.WriteRune('a')
			case strings.ContainsRune("éê", r):
				b.WriteRune('e')
			case r == 'í':
				b.WriteRune('i')
			case strings.ContainsRune("óôõ", r):
				b.WriteRune('o')
			case r == 'ú':
				b.WriteRune('u')
			case r == 'ç':
				b.WriteRune('c')
			default:
				b.WriteRune('-')
			}
		}
		out := strings.Trim(b.String(), "-")
		for strings.Contains(out, "--") {
			out = strings.ReplaceAll(out, "--", "-")
		}
		if len(out) > 40 {
			out = strings.Trim(out[:40], "-")
		}
		return out
	}
	return fmt.Sprintf("memoria-de-calculo-%s-%s.pdf", clean(p.ClientName), clean(p.Name))
}

// ---------- utilitários de desenho ----------

func (d *doc) text(c rgb)   { d.SetTextColor(c.r, c.g, c.b) }
func (d *doc) stroke(c rgb) { d.SetDrawColor(c.r, c.g, c.b) }
func (d *doc) fill(c rgb)   { d.SetFillColor(c.r, c.g, c.b) }

func (d *doc) font(family string, bold bool, size float64) {
	style := ""
	if bold {
		family, style = "Roboto", "B"
	}
	d.SetFont(family, style, size)
}

// put escreve um texto em (x, y), com y no topo da linha.
func (d *doc) put(x, y, w, h float64, s, align string) {
	d.SetXY(x, y)
	d.CellFormat(w, h, s, "", 0, align, false, 0, "")
}

func (d *doc) hline(x, y, w float64, c rgb, width float64) {
	d.stroke(c)
	d.SetLineWidth(width)
	d.Line(x, y, x+w, y)
}

// ensure abre uma página nova se não couberem h milímetros.
func (d *doc) ensure(h float64) {
	if d.GetY()+h > bottomY {
		d.AddPage()
		d.SetY(margin + 2)
	}
}

// section escreve o título de uma seção com o fio verde da marca.
func (d *doc) section(title string, x, w float64) {
	y := d.GetY()
	d.font("RobotoMedium", false, 11)
	d.text(ink)
	d.put(x, y, w, 6, title, "L")
	d.hline(x, y+7, w, rule, 0.2)
	d.hline(x, y+7, 14, brand, 0.7)
	d.SetY(y + 9.5)
}

// kv escreve uma linha de rótulo e valor e devolve a altura usada.
func (d *doc) kv(x, y, w float64, label, value string, strong, indent bool) float64 {
	const h = 5.4
	lx := x
	if indent {
		lx += 3
	}
	d.font("Roboto", strong, 8.6)
	if strong {
		d.text(ink)
	} else {
		d.text(muted)
	}
	d.put(lx, y, w*0.62, h, label, "L")
	d.font("Roboto", strong, 8.6)
	d.text(ink)
	d.put(x, y, w, h, value, "R")
	d.hline(x, y+h, w, rule, 0.1)
	return h
}

// ---------- formatação ----------

func group(intPart string) string {
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func number(v float64, digits int) string {
	s := fmt.Sprintf("%.*f", digits, math.Abs(v))
	intPart, frac, _ := strings.Cut(s, ".")
	out := group(intPart)
	if digits > 0 {
		out += "," + frac
	}
	if v < 0 && strings.Trim(s, "0.") != "" {
		out = "-" + out
	}
	return out
}

func money(v float64) string {
	if v < 0 && math.Abs(v) >= 0.005 {
		return "-R$ " + number(-v, 2)
	}
	return "R$ " + number(math.Abs(v), 2)
}

func minus(v float64) string { return "- " + money(math.Abs(v)) }

func pct(fraction float64) string { return number(fraction*100, 2) + "%" }

func pctMonth(fraction *float64) string {
	if fraction == nil {
		return "indisponível"
	}
	return pct(*fraction) + " a.m."
}

func compact(v float64) string {
	abs := math.Abs(v)
	sign := ""
	if v < 0 {
		sign = "-"
	}
	switch {
	case abs >= 1e6:
		return fmt.Sprintf("%sR$ %s mi", sign, number(abs/1e6, 1))
	case abs >= 1e3:
		return fmt.Sprintf("%sR$ %s mil", sign, number(abs/1e3, 0))
	}
	return sign + "R$ " + number(abs, 0)
}

func month(m *int) string {
	if m == nil {
		return "não recupera"
	}
	return fmt.Sprintf("mês %d", *m)
}

func brDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02/01/2006")
}

var statusLabel = map[string]string{
	store.StatusDraft: "Rascunho", store.StatusActive: "Ativo",
	store.StatusCompleted: "Concluído", store.StatusCancelled: "Cancelado",
}

var viabilityLabel = map[finance.Viability]string{
	finance.Viable: "Viável", finance.CommercialException: "Exceção comercial", finance.NotViable: "Inviável",
}

var viabilityColor = map[finance.Viability]rgb{
	finance.Viable: good, finance.CommercialException: warn, finance.NotViable: bad,
}

var healthLabel = map[finance.HealthStatus]string{
	finance.HealthExcellent: "excelente", finance.HealthGood: "boa", finance.HealthAdequate: "adequada",
	finance.HealthLimit: "no limite", finance.HealthCritical: "crítica",
}

var kindLabel = map[string]string{"hardware": "Hardware", "software": "Software", "points_license": "Licença por pontos"}

func saoPaulo(t time.Time) time.Time {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return t.UTC()
	}
	return t.In(loc)
}

// ---------- partes do documento ----------

func (d *doc) footer() {
	y := pageH - 12.0
	d.hline(margin, y-2, contW, rule, 0.2)
	d.font("Roboto", false, 7.5)
	d.text(muted)
	when := saoPaulo(d.in.GeneratedAt).Format("02/01/2006 às 15:04")
	d.put(margin, y, contW, 4, fmt.Sprintf("Informo. Documento de uso interno, gerado em %s por %s.", when, d.in.GeneratedBy), "L")
	d.put(margin, y, contW, 4, fmt.Sprintf("Página %d de {paginas}", d.PageNo()), "R")
}

func (d *doc) header() {
	p := d.in.Project
	const logoW = 44.0
	d.ImageOptions("logo", margin, margin, logoW, 0, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	d.font("Roboto", false, 9)
	d.text(muted)
	d.put(margin, margin-0.6, contW, 4.2, "Memória de cálculo de locação", "R")
	d.put(margin, margin+3.6, contW, 4.2, "Uso interno", "R")
	y := margin + logoW*logoRatio + 7
	d.hline(margin, y, contW, brand, 0.8)

	y += 6
	d.font("Roboto", true, 19)
	d.text(ink)
	d.SetXY(margin, y)
	d.MultiCell(contW, 8, p.ClientName, "", "L", false)
	d.font("RobotoLight", false, 12.5)
	d.text(ink)
	d.SetX(margin)
	d.MultiCell(contW, 6, p.Name, "", "L", false)

	meta := []string{statusLabel[p.Status], fmt.Sprintf("contrato de %d meses", p.DurationMonths)}
	if p.DealID != nil {
		meta = append(meta, fmt.Sprintf("deal %d do Pipedrive", *p.DealID))
	}
	if p.IsTest {
		meta = append(meta, "projeto de teste")
	}
	meta = append(meta, "responsável: "+p.OwnerName)
	d.font("Roboto", false, 8.6)
	d.text(muted)
	d.SetX(margin)
	d.MultiCell(contW, 5, strings.Join(meta, ", "), "", "L", false)
	d.SetY(d.GetY() + 4)
}

func (d *doc) verdictText() string {
	s := d.in.Scenarios.Pessimistic
	n := d.in.Project.DurationMonths
	switch s.Viability {
	case finance.Viable:
		return fmt.Sprintf("O investimento volta no mês %d de %d, dentro do limite de %d meses da política.", *s.PaybackMonth, n, s.MaxPaybackMonth)
	case finance.CommercialException:
		return fmt.Sprintf("O contrato cria valor, mas o investimento só volta no mês %d de %d. O limite da política é o mês %d.", *s.PaybackMonth, n, s.MaxPaybackMonth)
	}
	if s.PaybackMonth == nil {
		return fmt.Sprintf("O contrato não recupera o investimento em %d meses à taxa de oportunidade da empresa.", n)
	}
	return "O contrato não rende mais que a taxa de oportunidade da empresa."
}

func (d *doc) verdict() {
	s := d.in.Scenarios.Pessimistic
	y := d.GetY()
	d.font("Roboto", true, 24)
	d.text(viabilityColor[s.Viability])
	d.put(margin, y, contW, 10, viabilityLabel[s.Viability], "L")
	d.font("Roboto", false, 10.5)
	d.text(ink)
	d.SetXY(margin, y+11)
	d.MultiCell(contW, 5.2, d.verdictText(), "", "L", false)

	y = d.GetY() + 3
	d.hline(margin, y, contW, rule, 0.2)
	y += 2.5
	buffer := "nenhuma"
	if d.in.Health.HasPayback {
		buffer = fmt.Sprintf("%d meses", d.in.Health.BufferMonths)
	}
	irrNote := ""
	if d.in.IRR.Status == finance.IRRBelowThreshold {
		irrNote = "abaixo do mínimo de " + pct(d.in.IRR.Threshold)
	}
	facts := []struct{ label, value, note string }{
		{"Valor presente líquido", money(s.NPV), "a " + pct(d.in.Policy.OpportunityRate) + " ao mês"},
		{"Retorno do projeto (TIR)", pctMonth(s.MonthlyIRR), irrNote},
		{"Folga depois do payback", buffer, "saúde " + healthLabel[d.in.Health.Status]},
		{"Investimento inicial", money(s.Capex), ""},
	}
	colW := contW / float64(len(facts))
	for i, f := range facts {
		x := margin + float64(i)*colW
		d.font("Roboto", false, 8)
		d.text(muted)
		d.put(x, y, colW, 4, f.label, "L")
		d.font("RobotoMedium", false, 12.5)
		d.text(ink)
		d.put(x, y+4.4, colW, 6, f.value, "L")
		if f.note != "" {
			d.font("Roboto", false, 7.6)
			d.text(muted)
			if strings.HasPrefix(f.note, "abaixo") {
				d.text(warn)
			}
			d.put(x, y+10.4, colW, 4, f.note, "L")
		}
	}
	d.SetY(y + 17)
}

func niceStep(span float64, target int) float64 {
	raw := span / float64(target)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch norm := raw / mag; {
	case norm >= 5:
		return 10 * mag
	case norm >= 2:
		return 5 * mag
	case norm >= 1:
		return 2 * mag
	}
	return mag
}

// chart desenha o saldo descontado acumulado, o payback e o limite da política.
func (d *doc) chart() {
	s := d.in.Scenarios.Pessimistic
	points := make([]float64, 0, len(s.Months)+1)
	points = append(points, s.InitialFlow)
	for _, m := range s.Months {
		points = append(points, m.CumulativeDiscounted)
	}
	n := len(points) - 1
	lo, hi := 0.0, 0.0
	for _, v := range points {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	span := hi - lo
	if span == 0 {
		span = 1
	}
	step := niceStep(span, 4)
	yMin, yMax := math.Floor(lo/step)*step, math.Ceil(hi/step)*step

	const h, left, top = 50.0, 20.0, 6.0
	y0 := d.GetY()
	px, pw := margin+left, contW-left-2
	py, ph := y0+top, h-top-7
	xAt := func(m float64) float64 { return px + m/float64(n)*pw }
	yAt := func(v float64) float64 { return py + (1-(v-yMin)/(yMax-yMin))*ph }
	zero := yAt(0)

	d.font("Roboto", false, 7)
	d.text(muted)
	for v := yMin; v <= yMax+step/2; v += step {
		if math.Abs(v) < step/1e6 {
			v = 0
		}
		c, w := rule, 0.1
		if v == 0 {
			c, w = muted, 0.2
		}
		d.hline(px, yAt(v), pw, c, w)
		d.put(margin, yAt(v)-2, left-2, 4, compact(v), "R")
	}
	xStep := 12
	if n <= 12 {
		xStep = 3
	} else if n <= 24 {
		xStep = 6
	}
	for m := 0; m <= n; m += xStep {
		label := fmt.Sprintf("mês %d", m)
		if m == 0 {
			label = "início"
		}
		d.put(xAt(float64(m))-10, py+ph+1.5, 20, 4, label, "C")
	}

	// Áreas abaixo e acima de zero, segmento a segmento, cortando no cruzamento.
	area := func(x1, v1, x2, v2 float64) {
		c := good
		if v1 < 0 || v2 < 0 {
			c = bad
		}
		d.fill(c)
		d.Polygon([]fpdf.PointType{{X: x1, Y: zero}, {X: x1, Y: yAt(v1)}, {X: x2, Y: yAt(v2)}, {X: x2, Y: zero}}, "F")
	}
	d.SetAlpha(0.12, "Normal")
	for i := 0; i < n; i++ {
		x1, x2, v1, v2 := xAt(float64(i)), xAt(float64(i+1)), points[i], points[i+1]
		if (v1 < 0) != (v2 < 0) && v1 != 0 && v2 != 0 {
			xc := x1 + (x2-x1)*(-v1)/(v2-v1)
			area(x1, v1, xc, 0)
			area(xc, 0, x2, v2)
			continue
		}
		area(x1, v1, x2, v2)
	}
	d.SetAlpha(1, "Normal")

	limit := s.MaxPaybackMonth
	payback := s.PaybackMonth
	near := payback != nil && math.Abs(xAt(float64(*payback))-xAt(float64(limit))) < 42
	paybackLeft := payback != nil && ((near && *payback <= limit) || (!near && xAt(float64(*payback)) > px+pw-40))
	if limit > 0 && limit <= n {
		x := xAt(float64(limit))
		d.stroke(muted)
		d.SetLineWidth(0.2)
		d.SetDashPattern([]float64{1, 1}, 0)
		d.Line(x, py-3, x, py+ph)
		d.SetDashPattern([]float64{}, 0)
		limitLeft := (near && !paybackLeft) || (!near && x > px+pw-48)
		d.font("Roboto", false, 7)
		d.text(muted)
		label := fmt.Sprintf("Limite da política: mês %d", limit)
		if limitLeft {
			d.put(x-51, py-5.5, 50, 4, label, "R")
		} else {
			d.put(x+1, py-5.5, 50, 4, label, "L")
		}
	}

	d.stroke(ink)
	d.SetLineWidth(0.45)
	d.SetLineJoinStyle("round")
	for i := 0; i < n; i++ {
		d.Line(xAt(float64(i)), yAt(points[i]), xAt(float64(i+1)), yAt(points[i+1]))
	}

	if payback != nil && *payback <= n {
		x, y := xAt(float64(*payback)), yAt(points[*payback])
		c := good
		if *payback > limit {
			c = amber
		}
		d.fill(rgb{255, 255, 255})
		d.Circle(x, y, 1.9, "F")
		d.fill(c)
		d.Circle(x, y, 1.4, "F")
		d.font("RobotoMedium", false, 8)
		d.text(ink)
		label := fmt.Sprintf("Payback: mês %d", *payback)
		if paybackLeft {
			d.put(x-43, math.Max(y-6.5, py), 40, 4, label, "R")
		} else {
			d.put(x+3, math.Min(y+2.5, py+ph-4.5), 40, 4, label, "L")
		}
	}
	d.font("Roboto", false, 7.4)
	d.text(muted)
	d.put(margin, y0+h, contW, 4, "Saldo de caixa acumulado, com cada mês descontado pela taxa de oportunidade.", "L")
	d.SetY(y0 + h + 9)
}

func (d *doc) conditionsAndLedger() {
	p := d.in.Project
	s := d.in.Scenarios.Pessimistic
	const gap = 8.0
	colW := (contW - gap) / 2
	rightX := margin + colW + gap
	d.ensure(78)
	top := d.GetY()

	d.section("Condições do contrato", margin, colW)
	y := d.GetY()
	fx := "não informado"
	if p.ExchangeRate != nil {
		fx = number(*p.ExchangeRate, 4)
	}
	start := "não informado"
	if p.ContractStartDate != nil {
		start = brDate(*p.ContractStartDate)
	}
	left := [][2]string{
		{"Mensalidade", money(p.MonthlyRevenue)},
		{"Prazo", fmt.Sprintf("%d meses", p.DurationMonths)},
		{"Receita de implantação", money(p.ImplementationRevenue)},
		{"Inadimplência esperada", pct(p.ExpectedLossRate)},
		{"Suporte por mês", money(p.MonthlySupportCost)},
		{"Câmbio USD/BRL", fx},
		{"Início do contrato", start},
		{"UF de destino", p.DestState},
	}
	for _, r := range left {
		y += d.kv(margin, y, colW, r[0], r[1], false, false)
	}
	y += 5
	d.SetY(y)
	d.section("Investimento e comissões", margin, colW)
	y = d.GetY()
	c := s.Commission
	inv := []struct {
		label, value string
		strong       bool
	}{
		{"Hardware", money(d.in.Capex.Hardware), false},
		{"Software e licenças", money(d.in.Capex.Software), false},
		{"DIFAL", money(d.in.Capex.Difal), false},
		{"Investimento inicial (CAPEX)", money(d.in.Capex.Total), true},
		{fmt.Sprintf("Comissão do vendedor (%s)", pct(c.Rates.Seller)), money(c.Seller), false},
		{fmt.Sprintf("Comissão do arquiteto (%s)", pct(c.Rates.Architect)), money(c.Architect), false},
		{fmt.Sprintf("Comissão do gerente (%s)", pct(c.Rates.Manager)), money(c.Manager), false},
		{"Comissões", money(c.Total), true},
	}
	for _, r := range inv {
		y += d.kv(margin, y, colW, r.label, r.value, r.strong, false)
	}
	leftEnd := y

	d.SetY(top)
	d.section("Memória de cálculo", rightX, colW)
	y = d.GetY()
	taxRate := 0.0
	if s.TotalRevenue > 0 {
		taxRate = s.Taxes / s.TotalRevenue
	}
	ledger := []struct {
		label, value   string
		strong, indent bool
	}{
		{"Receita bruta contratada", money(s.TotalRevenue), false, false},
		{fmt.Sprintf("Impostos estimados (%s)", pct(taxRate)), minus(s.Taxes), false, true},
		{"Inadimplência esperada", minus(s.TotalExpectedLoss), false, true},
		{"Suporte", minus(s.TotalSupportCost), false, true},
		{"Licenças por pontos", minus(s.TotalLicensingCost), false, true},
		{"Comissões", minus(c.Total), false, true},
		{"Caixa gerado pelo contrato", money(s.NetCashAfterRisks), true, false},
		{"Investimento inicial (CAPEX)", minus(s.Capex), false, true},
		{"Resultado líquido de caixa", money(s.NetCashResult), true, false},
	}
	for _, r := range ledger {
		y += d.kv(rightX, y, colW, r.label, r.value, r.strong, r.indent)
	}
	y += 5
	d.SetY(y)
	d.section("Indicadores", rightX, colW)
	y = d.GetY()
	ind := [][2]string{
		{"Valor presente líquido (VPL)", money(s.NPV)},
		{"VPL sem inadimplência e suporte", money(d.in.Scenarios.Base.NPV)},
		{"Retorno do projeto (TIR)", pctMonth(s.MonthlyIRR)},
		{"Retorno sobre o investimento", pct(s.ROIOnCapex)},
		{"Payback descontado", month(s.PaybackMonth)},
		{"Payback sem desconto", month(s.SimplePaybackMonth)},
		{"Caixa gerado depois do payback", money(s.CashAfterPayback)},
	}
	for _, r := range ind {
		y += d.kv(rightX, y, colW, r[0], r[1], false, false)
	}
	d.SetY(math.Max(leftEnd, y) + 7)
}

// column descreve uma coluna de tabela.
type column struct {
	title string
	w     float64
	align string
}

func (d *doc) tableHead(cols []column) {
	y := d.GetY()
	d.font("Roboto", false, 7.6)
	d.text(muted)
	x := margin
	for _, c := range cols {
		d.put(x+1, y, c.w-2, 5, c.title, c.align)
		x += c.w
	}
	d.hline(margin, y+5.4, contW, rule, 0.25)
	d.SetY(y + 6.2)
}

// tableRow escreve uma linha; a primeira célula pode ter uma segunda linha.
func (d *doc) tableRow(cols []column, cells []string, sub string, strong bool, tint *rgb) {
	const lineH = 4.6
	h := lineH + 1.2
	if sub != "" {
		h += 3.8
	}
	if d.GetY()+h > bottomY {
		d.AddPage()
		d.SetY(margin + 2)
		d.tableHead(cols)
	}
	y := d.GetY()
	if tint != nil {
		d.fill(*tint)
		d.Rect(margin, y-0.4, contW, h, "F")
	}
	x := margin
	for i, c := range cols {
		d.font("Roboto", strong, 8.6)
		d.text(ink)
		d.put(x+1, y, c.w-2, lineH, cells[i], c.align)
		x += c.w
	}
	if sub != "" {
		d.font("Roboto", false, 7.4)
		d.text(muted)
		d.put(margin+1, y+lineH-0.4, contW-2, 3.8, sub, "L")
	}
	d.hline(margin, y+h-0.4, contW, rule, 0.1)
	d.SetY(y + h)
}

func (d *doc) items() {
	d.ensure(30)
	d.section("Itens da proposta", margin, contW)
	cols := []column{
		{"Item", 68, "L"}, {"Qtd.", 12, "R"}, {"Preço unitário", 28, "R"}, {"DIFAL", 40, "R"}, {"Total", 30, "R"},
	}
	d.tableHead(cols)
	costs := d.in.Capex.Items
	ci := 0
	for _, it := range d.in.Items {
		sub := kindLabel[it.Kind]
		if it.Kind == "points_license" {
			if it.License != nil {
				sub += fmt.Sprintf(", %s: %s ponto(s) por dia a US$ %s por ponto", it.License.ModelName, number(it.License.DailyPoints, 2), number(it.License.PriceUSD, 2))
			}
			d.tableRow(cols, []string{it.Name, fmt.Sprint(it.Quantity), "recorrente", "", "custo mensal"}, sub, false, nil)
			continue
		}
		if it.Kind == "hardware" && it.OriginState != nil {
			sub += ", origem " + *it.OriginState
			if it.Imported {
				sub += ", importado"
			}
		}
		unit := money(it.UnitPrice)
		if it.Currency == "USD" {
			unit = "US$ " + number(it.UnitPrice, 2)
		}
		difal, total := "", ""
		if ci < len(costs) {
			cost := costs[ci]
			ci++
			total = money(cost.TotalBRL)
			if cost.Difal != nil && cost.Difal.Value > 0 {
				difal = fmt.Sprintf("%s (%s)", money(cost.Difal.Value), pct(cost.Difal.Rate))
			}
		}
		d.tableRow(cols, []string{it.Name, fmt.Sprint(it.Quantity), unit, difal, total}, sub, false, nil)
	}
	d.tableRow(cols, []string{"Investimento inicial (CAPEX)", "", "", money(d.in.Capex.Difal), money(d.in.Capex.Total)}, "", true, &soft)
	if d.in.Licensing.TotalCostBRL > 0 {
		d.tableRow(cols, []string{"Licenças por pontos no contrato (custo recorrente)", "", "", "", money(d.in.Licensing.TotalCostBRL)}, "", false, nil)
	}
	d.SetY(d.GetY() + 7)
}

func (d *doc) pricing() {
	p := d.in.Project
	s := d.in.Scenarios.Pessimistic
	d.ensure(52)
	const gap = 8.0
	leftW := 62.0
	rightX := margin + leftW + gap
	rightW := contW - leftW - gap
	top := d.GetY()

	d.section("Mensalidade", margin, leftW)
	y := d.GetY()
	floor := func(v *float64) string {
		if v == nil {
			return "nenhuma resolve"
		}
		return money(*v)
	}
	y += d.kv(margin, y, leftW, "Proposta", money(p.MonthlyRevenue), false, false)
	y += d.kv(margin, y, leftW, fmt.Sprintf("Mínima da política (mês %d)", s.MaxPaybackMonth), floor(s.MinimumPolicyMonthly), false, false)
	y += d.kv(margin, y, leftW, "Mínima para VPL zero", floor(s.MinimumEconomicMonthly), false, false)
	if s.MinimumPolicyMonthly != nil {
		gapV := p.MonthlyRevenue - *s.MinimumPolicyMonthly
		label := "Folga sobre o mínimo da política"
		if gapV < 0 {
			label = "Falta para o mínimo da política"
		}
		y += d.kv(margin, y, leftW, label, money(math.Abs(gapV)), true, false)
	}
	leftEnd := y

	d.SetY(top)
	d.section("Sensibilidade à mensalidade", rightX, rightW)
	y = d.GetY()
	widths := []float64{15, 23, 25, 18, rightW - 81}
	heads := []string{"Variação", "Mensalidade", "VPL", "Payback", "Decisão"}
	aligns := []string{"L", "R", "R", "R", "L"}
	d.font("Roboto", false, 7.6)
	d.text(muted)
	x := rightX
	for i, hd := range heads {
		d.put(x+1, y, widths[i]-2, 4.6, hd, aligns[i])
		x += widths[i]
	}
	y += 5
	d.hline(rightX, y, rightW, rule, 0.25)
	for _, r := range d.in.Sensitivity {
		label := "Proposta"
		if r.Delta != 0 {
			label = fmt.Sprintf("%+.0f%%", r.Delta*100)
		}
		if r.Delta == 0 {
			d.fill(soft)
			d.Rect(rightX, y, rightW, 5.2, "F")
		}
		cells := []string{label, money(r.MonthlyRevenue), money(r.NPV), month(r.PaybackMonth), viabilityLabel[r.Viability]}
		x = rightX
		for i, c := range cells {
			d.font("Roboto", r.Delta == 0, 8.4)
			d.text(ink)
			if i == 4 {
				d.text(viabilityColor[r.Viability])
			}
			d.put(x+1, y+0.4, widths[i]-2, 4.4, c, aligns[i])
			x += widths[i]
		}
		y += 5.2
		d.hline(rightX, y, rightW, rule, 0.1)
	}
	d.SetY(math.Max(leftEnd, y) + 7)
}

func (d *doc) monthly() {
	s := d.in.Scenarios.Pessimistic
	// O fluxo continua na página em que estiver e quebra sozinho; só começa
	// em página nova se não couberem o título e algumas linhas.
	d.ensure(45)
	d.section("Fluxo de caixa mês a mês", margin, contW)
	cols := []column{
		{"Mês", 14, "L"}, {"Receita", 27, "R"}, {"Impostos", 27, "R"}, {"Perda e suporte", 27, "R"},
		{"Licenças", 25, "R"}, {"Fluxo líquido", 29, "R"}, {"Saldo descontado", 29, "R"},
	}
	d.tableHead(cols)
	d.tableRow(cols, []string{"Início", "", "", "", "", money(s.InitialFlow), money(s.InitialFlow)}, "", false, nil)
	for _, m := range s.Months {
		var tint *rgb
		strong := false
		if s.PaybackMonth != nil && m.Month == *s.PaybackMonth {
			tint, strong = &soft, true
		}
		d.tableRow(cols, []string{
			fmt.Sprint(m.Month), money(m.GrossRevenue), money(m.Taxes), money(m.ExpectedLoss + m.SupportCost),
			money(m.LicensingCost), money(m.NetCashFlow), money(m.CumulativeDiscounted),
		}, "", strong, tint)
	}
	d.font("Roboto", false, 7.4)
	d.text(muted)
	y := d.GetY() + 1.5
	note := "No início entram o investimento, as comissões e a receita de implantação líquida de impostos."
	if s.PaybackMonth != nil {
		note += " A linha destacada é o mês do payback."
	}
	d.put(margin, y, contW, 4, note, "L")
	d.SetY(y + 10)
}

func (d *doc) assumptions() {
	pol := d.in.Policy
	lines := []string{
		fmt.Sprintf("Política financeira versão %d: impostos de %s sobre a receita, taxa de oportunidade de %s ao mês e TIR mínima de %s ao mês.",
			d.in.SettingsVersionID, pct(pol.TaxRate), pct(pol.OpportunityRate), pct(pol.IRRAlertThreshold)),
		"A decisão usa o cenário pessimista, com a inadimplência esperada e o custo de suporte informados no projeto.",
		"O payback é o mês a partir do qual o saldo descontado não volta a ficar negativo até o fim do contrato.",
		"A alíquota de impostos é uma premissa de simulação para IBS e CBS, não uma apuração fiscal.",
		"O DIFAL é uma estimativa pela diferença entre a alíquota interna do destino e a interestadual. Não considera base dupla, FCP, benefícios fiscais nem NCM.",
		"As comissões são calculadas sobre a margem do contrato após impostos e consideradas pagas no início do contrato.",
	}
	if d.in.Licensing.TotalCostBRL > 0 {
		lines = append(lines, "Licenças por pontos são estimativas de consumo por dias corridos, com preço e consumo congelados quando o item entrou na proposta.")
	}
	d.ensure(12 + float64(len(lines))*9)
	d.section("Premissas e ressalvas", margin, contW)
	d.font("Roboto", false, 8.4)
	d.text(ink)
	for _, l := range lines {
		y := d.GetY()
		d.fill(brand)
		d.Circle(margin+1.2, y+2.2, 0.6, "F")
		d.SetXY(margin+4, y)
		d.MultiCell(contW-4, 4.4, l, "", "L", false)
		d.SetY(d.GetY() + 1.2)
	}
}
