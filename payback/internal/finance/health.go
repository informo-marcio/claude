package finance

import "math"

// HealthStatus é a qualidade do contrato pela folga após o payback.
type HealthStatus string

const (
	HealthExcellent HealthStatus = "EXCELENTE"
	HealthGood      HealthStatus = "BOA"
	HealthAdequate  HealthStatus = "ADEQUADA"
	HealthLimit     HealthStatus = "LIMITE"
	HealthCritical  HealthStatus = "CRITICA"
)

// Health mede quanto do contrato resta depois de recuperar o investimento.
type Health struct {
	Status HealthStatus `json:"status"`
	// Score é a parcela do prazo que resta após o payback, de 0 a 100.
	Score        int  `json:"score"`
	BufferMonths int  `json:"bufferMonths"`
	HasPayback   bool `json:"hasPayback"`
}

// ContractHealth avalia a saúde de um contrato a partir do payback descontado
// sustentado do cenário pessimista.
func ContractHealth(durationMonths int, paybackMonth *int) Health {
	if durationMonths <= 0 || paybackMonth == nil || *paybackMonth > durationMonths {
		return Health{Status: HealthCritical}
	}
	buffer := durationMonths - *paybackMonth
	score := int(math.Floor(float64(buffer)/float64(durationMonths)*100 + 0.5))
	return Health{Status: healthFromScore(score), Score: score, BufferMonths: buffer, HasPayback: true}
}

func healthFromScore(score int) HealthStatus {
	switch {
	case score >= 60:
		return HealthExcellent
	case score >= 45:
		return HealthGood
	case score >= 30:
		return HealthAdequate
	case score >= 15:
		return HealthLimit
	default:
		return HealthCritical
	}
}

// IRRStatus é a situação da TIR frente ao limite de alerta.
type IRRStatus string

const (
	IRRAboveThreshold IRRStatus = "ACIMA_DO_LIMITE"
	IRRBelowThreshold IRRStatus = "ABAIXO_DO_LIMITE"
	IRRUnavailable    IRRStatus = "NAO_CALCULAVEL"
)

// IRRAssessment compara a TIR mensal com o limite de alerta da política.
type IRRAssessment struct {
	Status    IRRStatus `json:"status"`
	Threshold float64   `json:"threshold"`
	// Difference é TIR − limite, como fração; nil sem TIR.
	Difference *float64 `json:"difference"`
}

// AssessIRR classifica a TIR. TIR igual ao limite atende.
func AssessIRR(monthlyIRR *float64, threshold float64) IRRAssessment {
	if monthlyIRR == nil {
		return IRRAssessment{Status: IRRUnavailable, Threshold: threshold}
	}
	diff := roundRate(*monthlyIRR - threshold)
	status := IRRAboveThreshold
	if diff < 0 {
		status = IRRBelowThreshold
	}
	return IRRAssessment{Status: status, Threshold: threshold, Difference: &diff}
}
