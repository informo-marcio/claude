package finance

import "math"

const epsilon = 2.220446049250313e-16

// roundTo arredonda meio para cima em n casas, igual ao motor anterior
// (Math.round((v + EPSILON) * 10^n) / 10^n), para manter os mesmos centavos.
func roundTo(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Floor((v+epsilon)*p+0.5) / p
}

// roundCents arredonda um valor monetário a centavos.
func roundCents(v float64) float64 { return roundTo(v, 2) }

// roundRate arredonda uma fração a seis casas (quatro casas do percentual).
func roundRate(v float64) float64 { return roundTo(v, 6) }

// annuityFactor é o valor presente de uma anuidade unitária paga ao fim de
// cada mês.
func annuityFactor(months int, rate float64) float64 {
	if months <= 0 {
		return 0
	}
	if rate == 0 {
		return float64(months)
	}
	return (1 - math.Pow(1+rate, -float64(months))) / rate
}
