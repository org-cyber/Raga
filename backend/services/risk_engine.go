package services

import "log"

// TransactionData represents the data needed for scoring
type TransactionData struct {
	UserID        string
	TransactionID string
	Amount        float64
	Currency      string
	IPAddress     string
	DeviceID      string
	Location      string
}

// RiskResult is what we return after full scoring (rule-based + AI)
type RiskResult struct {
	Score              int      `json:"score"`
	Level              string   `json:"level"`
	Reasons            []string `json:"reasons"`
	AITriggered        bool     `json:"ai_triggered"`
	AIConfidence       float64  `json:"ai_confidence,omitempty"`
	AISummary          string   `json:"ai_summary,omitempty"`
	AIRecommendation   string   `json:"ai_recommendation,omitempty"`
	AIFraudProbability float64  `json:"ai_fraud_probability,omitempty"`
}

// CalculateRisk runs the rule-based engine, then calls Groq AI if the score is high enough.
func CalculateRisk(tx TransactionData) RiskResult {

	reasons := []string{}

	// -------------------------------------------------------
	// RULE-BASED SCORING
	// Each rule produces a risk value between 0.0 and 1.0.
	// Weights must sum to 1.0 so the final score is a true percentage.
	// -------------------------------------------------------

	// Rule 1: Transaction Amount (weight: 35%)
	// Tiered — higher amounts carry more risk
	amountRisk := 0.0
	switch {
	case tx.Amount > 500000:
		amountRisk = 1.0
		reasons = append(reasons, "Very high transaction amount (>500k)")
	case tx.Amount > 100000:
		amountRisk = 0.6
		reasons = append(reasons, "High transaction amount (>100k)")
	case tx.Amount > 50000:
		amountRisk = 0.3
		reasons = append(reasons, "Elevated transaction amount (>50k)")
	}

	// Rule 2: Currency (weight: 20%)
	// Non-NGN transactions are higher risk in this context
	currencyRisk := 0.0
	if tx.Currency != "NGN" {
		currencyRisk = 1.0
		reasons = append(reasons, "Foreign currency transaction ("+tx.Currency+")")
	}

	// Rule 3: Device ID (weight: 15%)
	// Missing device = anonymous = higher risk
	deviceRisk := 0.0
	if tx.DeviceID == "" {
		deviceRisk = 1.0
		reasons = append(reasons, "Missing device ID")
	}

	// Rule 4: IP Address (weight: 15%)
	// Missing IP = can't trace origin
	ipRisk := 0.0
	if tx.IPAddress == "" {
		ipRisk = 1.0
		reasons = append(reasons, "Missing IP address")
	}

	// Rule 5: Location (weight: 15%)
	// Missing location = unverifiable origin
	locationRisk := 0.0
	if tx.Location == "" {
		locationRisk = 1.0
		reasons = append(reasons, "Missing location")
	}

	// -------------------------------------------------------
	// WEIGHTED FINAL SCORE (weights sum to exactly 1.0)
	// -------------------------------------------------------
	score := (amountRisk * 0.35) +
		(currencyRisk * 0.20) +
		(deviceRisk * 0.15) +
		(ipRisk * 0.15) +
		(locationRisk * 0.15)

	// Convert to integer percentage (0–100)
	finalScore := int(score * 100)

	// -------------------------------------------------------
	// RISK LEVEL from rule-based score alone
	// -------------------------------------------------------
	level := "LOW"
	if finalScore >= 70 {
		level = "HIGH"
	} else if finalScore >= 40 {
		level = "MEDIUM"
	}

	// -------------------------------------------------------
	// AI GATE: Call Groq AI when score >= 40 (MEDIUM or above)
	// The AI can upgrade or confirm the risk level.
	// -------------------------------------------------------
	aiTriggered := false
	var aiResult AIResult

	if finalScore >= 40 {
		aiTriggered = true
		log.Printf("[AI GATE] Score=%d for txn=%s — calling Groq AI...", finalScore, tx.TransactionID)

		result, err := AnalyzeTransaction(tx, finalScore)
		if err != nil {
			// AI failed — log it clearly and escalate to HIGH for safety
			log.Printf("[AI ERROR] txn=%s: %v", tx.TransactionID, err)
			level = "HIGH"
			reasons = append(reasons, "AI analysis unavailable — escalated to HIGH for manual review")
		} else {
			aiResult = result
			log.Printf("[AI OK] txn=%s confidence=%.2f action=%s", tx.TransactionID, result.Confidence, result.RecommendedAction)

			// Let AI upgrade the risk level if it's more confident
			switch result.RecommendedAction {
			case "BLOCK":
				level = "HIGH"
				reasons = append(reasons, "AI recommends BLOCK: "+result.Reasoning)
			case "REVIEW":
				if level == "LOW" {
					level = "MEDIUM" // AI can upgrade LOW → MEDIUM
				}
				reasons = append(reasons, "AI recommends REVIEW: "+result.Reasoning)
			case "APPROVE":
				// AI is confident it's safe — don't downgrade below rule-based level,
				// but note the AI's opinion
				reasons = append(reasons, "AI recommends APPROVE: "+result.Reasoning)
			}
		}
	} else {
		log.Printf("[AI GATE] Score=%d for txn=%s — below threshold, skipping AI", finalScore, tx.TransactionID)
	}

	return RiskResult{
		Score:              finalScore,
		Level:              level,
		Reasons:            reasons,
		AITriggered:        aiTriggered,
		AIConfidence:       aiResult.Confidence,
		AISummary:          aiResult.Reasoning,
		AIRecommendation:   aiResult.RecommendedAction,
		AIFraudProbability: aiResult.FraudProbability,
	}
}
