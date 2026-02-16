package services

// This struct represents the data needed for scoring
type TransactionData struct {
	UserID        string
	TransactionID string
	Amount        float64
	Currency      string
	IPAddress     string
	DeviceID      string
}

// This is what we return after scoring
type RiskResult struct {
	Score        int      `json:"score"`
	Level        string   `json:"level"`
	Reasons      []string `json:"reasons"`
	AIConfidence float64  `json:"ai_confidence,omitempty"` // optional for now
	AISummary    string   `json:"ai_summary,omitempty"`    // optional for now

}

type AiResult struct {
	Confidence float64
	Summary    string
}

// This function calculates risk
func CalculateRisk(tx TransactionData) RiskResult {

	score := 0.0
	reasons := []string{}
	amountWeight := 0.4
	currencyWeight := 0.2
	deviceWeight := 0.2
	ipWeight := 0.2

	// Rule 1: Large amount
	amountRisk := 0.0
	if tx.Amount > 100000 {
		amountRisk += 1.0
		reasons = append(reasons, "High transaction amount")
	}

	// Rule 2: Foreign currency
	currencyRisk := 0.0
	if tx.Currency != "NGN" {
		currencyRisk += 1.0
		reasons = append(reasons, "Foreign currency")
	}

	// --- DEVICE RISK (placeholder) ---
	deviceRisk := 0.0
	if tx.DeviceID == "" {
		deviceRisk = 1.0
		reasons = append(reasons, "Missing device ID")
	}

	// --- IP RISK (placeholder) ---
	ipRisk := 0.0
	if tx.IPAddress == "" {
		ipRisk = 1.0
		reasons = append(reasons, "Missing IP address")
	}

	// --- FINAL WEIGHTED SCORE ---
	score = (amountRisk * amountWeight) +
		(currencyRisk * currencyWeight) +
		(deviceRisk * deviceWeight) +
		(ipRisk * ipWeight)

	// Convert to percentage
	finalScore := int(score * 100)

	level := "LOW"
	if finalScore >= 70 {
		level = "HIGH"
	} else if finalScore >= 40 {
		level = "MEDIUM"
	}

	// --- AI GATE (threshold 50) ---
	var aiResult AIResult
	if finalScore >= 50 {
		// Placeholder AI call
		aiResult = AnalyzeTransaction(tx, finalScore)

	}

	return RiskResult{
		Score:        finalScore,
		Level:        level,
		Reasons:      reasons,
		AIConfidence: aiResult.Confidence,
		AISummary:    aiResult.Summary,
	}
}
