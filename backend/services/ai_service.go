package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// AIResult represents the structured output from Groq AI
type AIResult struct {
	FraudProbability  float64 `json:"fraud_probability"`
	RecommendedAction string  `json:"recommended_action"`
	Reasoning         string  `json:"reasoning"`
	Confidence        float64 `json:"confidence"`
}

type groqRequest struct {
	Model       string        `json:"model"`
	Messages    []groqMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// AnalyzeTransaction sends transaction data to Groq AI and returns a structured risk assessment.
func AnalyzeTransaction(tx TransactionData, baselineScore int) (AIResult, error) {

	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return AIResult{}, fmt.Errorf("GROQ_API_KEY is not set in environment")
	}

	// System prompt: tells the AI its role and output format strictly
	systemPrompt := `You are a financial fraud detection AI for a Nigerian fintech platform.
Your job is to assess whether a transaction is fraudulent based on the data provided.
You MUST respond with ONLY a valid JSON object — no markdown, no explanation outside the JSON.
The JSON must follow this exact schema:
{
  "fraud_probability": <float between 0.0 and 1.0>,
  "recommended_action": <"APPROVE" | "REVIEW" | "BLOCK">,
  "reasoning": <one concise sentence explaining your decision>,
  "confidence": <float between 0.0 and 1.0>
}`

	// User prompt: the actual transaction details
	userPrompt := fmt.Sprintf(`Assess this transaction for fraud risk:

Transaction ID : %s
Amount         : %.2f %s
Location       : %s
Device ID      : %s
IP Address     : %s
Baseline Score : %d/100 (rule-based engine score, higher = riskier)

Respond with JSON only.`,
		tx.TransactionID,
		tx.Amount,
		tx.Currency,
		tx.Location,
		tx.DeviceID,
		tx.IPAddress,
		baselineScore,
	)

	body := groqRequest{
		// llama-3.3-70b-versatile is Groq's current recommended model (mixtral is deprecated)
		Model:       "llama-3.3-70b-versatile",
		Temperature: 0.1, // Low temperature = more deterministic, consistent JSON output
		MaxTokens:   256, // We only need a small JSON blob
		Messages: []groqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return AIResult{}, fmt.Errorf("failed to marshal Groq request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return AIResult{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	// 10 second timeout — generous enough for LLM, tight enough to not hang the API
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AIResult{}, fmt.Errorf("Groq HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	var groqResp groqResponse
	if err := json.NewDecoder(resp.Body).Decode(&groqResp); err != nil {
		return AIResult{}, fmt.Errorf("failed to decode Groq response: %w", err)
	}

	// Check if Groq returned an API-level error (e.g. invalid key, rate limit)
	if groqResp.Error != nil {
		return AIResult{}, fmt.Errorf("Groq API error: %s", groqResp.Error.Message)
	}

	if len(groqResp.Choices) == 0 {
		return AIResult{}, fmt.Errorf("Groq returned no choices")
	}

	rawContent := strings.TrimSpace(groqResp.Choices[0].Message.Content)

	// Strip markdown code fences if the model wraps JSON in ```json ... ```
	rawContent = strings.TrimPrefix(rawContent, "```json")
	rawContent = strings.TrimPrefix(rawContent, "```")
	rawContent = strings.TrimSuffix(rawContent, "```")
	rawContent = strings.TrimSpace(rawContent)

	var aiResult AIResult
	if err := json.Unmarshal([]byte(rawContent), &aiResult); err != nil {
		return AIResult{}, fmt.Errorf("AI returned invalid JSON (%q): %w", rawContent, err)
	}

	// Validate the recommended action is one of the expected values
	switch aiResult.RecommendedAction {
	case "APPROVE", "REVIEW", "BLOCK":
		// valid
	default:
		return AIResult{}, fmt.Errorf("AI returned unexpected action: %q", aiResult.RecommendedAction)
	}

	return aiResult, nil
}
