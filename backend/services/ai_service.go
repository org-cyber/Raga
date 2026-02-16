package services

import (
	"fmt"
	"time"
)

// AIResult represents the AI layer output
type AIResult struct {
	Confidence float64
	Summary    string
}

// AnalyzeTransaction is a placeholder for Groq AI integration
func AnalyzeTransaction(tx TransactionData, baselineScore int) AIResult {
	// For now, we simulate AI reasoning
	// Later, replace this with an actual API call to Groq

	// Simulate processing delay
	time.Sleep(50 * time.Millisecond)

	summary := "No AI integration yet"
	confidence := 0.0

	// Example logic for simulation
	if baselineScore >= 50 {
		summary = "Simulated AI: anomaly detected due to high amount and foreign currency"
		confidence = 0.85
	}

	fmt.Printf("AI analyzed transaction %s, baseline score %d\n", tx.TransactionID, baselineScore)

	return AIResult{
		Confidence: confidence,
		Summary:    summary,
	}
}
