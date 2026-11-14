package finance

// Commission é o detalhamento da comissão de uma proposta.
type Commission struct {
	Rates CommissionRates `json:"rates"`
	// ContractRevenue é mensalidade × prazo, sem a implantação.
	ContractRevenue float64 `json:"contractRevenue"`
	// CapexDeduction e LicensingDeduction são os custos abatidos da base.
	CapexDeduction     float64 `json:"capexDeduction"`
	LicensingDeduction float64 `json:"licensingDeduction"`
	// GrossBase é a margem contratual, limitada a zero.
	GrossBase float64 `json:"grossBase"`
	// Taxes é a carga tributária aplicada à base.
	Taxes float64 `json:"taxes"`
	// NetBase é a base sobre a qual as taxas incidem.
	NetBase   float64 `json:"netBase"`
	Seller    float64 `json:"seller"`
	Architect float64 `json:"architect"`
	Manager   float64 `json:"manager"`
	Total     float64 `json:"total"`
}

// computeCommission calcula a comissão sobre a margem contratual após
// impostos: (mensalidade × prazo − CAPEX − licenças por pontos) × (1 − imposto).
// Margem negativa não gera comissão. O valor é pago integralmente no mês zero.
func computeCommission(monthly float64, months int, capex, licensing, taxRate float64, rates CommissionRates) Commission {
	contract := monthly * float64(months)
	gross := contract - capex - licensing
	if gross < 0 {
		gross = 0
	}
	taxes := gross * taxRate
	net := gross - taxes
	seller := roundCents(net * rates.Seller)
	architect := roundCents(net * rates.Architect)
	manager := roundCents(net * rates.Manager)
	return Commission{
		Rates:              rates,
		ContractRevenue:    roundCents(contract),
		CapexDeduction:     roundCents(capex),
		LicensingDeduction: roundCents(licensing),
		GrossBase:          roundCents(gross),
		Taxes:              roundCents(taxes),
		NetBase:            roundCents(net),
		Seller:             seller,
		Architect:          architect,
		Manager:            manager,
		Total:              roundCents(net*rates.Seller + net*rates.Architect + net*rates.Manager),
	}
}
