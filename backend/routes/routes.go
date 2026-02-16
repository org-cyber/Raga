package routes

import (
	"asguard/middleware"
	"asguard/services"

	"github.com/gin-gonic/gin"
)

type TransactionRequest struct {
	UserID        string  `json:"user_id" binding:"required"`
	TransactionID string  `json:"transaction_id" binding:"required"`
	Amount        float64 `json:"amount" binding:"required"`
	Currency      string  `json:"currency" binding:"required"`
	IPAddress     string  `json:"ip_address" binding:"required"`
	DeviceID      string  `json:"device_id" binding:"required"`
	SimID         string  `json:"sim_id" binding:"required"`
	Timestamp     string  `json:"timestamp" binding:"required"`
}

func AnalyzeTransaction(c *gin.Context) {

	var req TransactionRequest

	// Bind incoming JSON into struct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"error": "invalid request payload",
		})
		return
	}

	// Call service layer AFTER validation succeeds
	riskResult := services.CalculateRisk(services.TransactionData{
		UserID:        req.UserID,
		TransactionID: req.TransactionID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		IPAddress:     req.IPAddress,
		DeviceID:      req.DeviceID,
	})

	// Return structured response
	c.JSON(200, gin.H{
		"transaction_id": req.TransactionID,
		"risk_score":     riskResult.Score,
		"risk_level":     riskResult.Level,
		"reasons":        riskResult.Reasons,
		"ai_confidence":  riskResult.AIConfidence,
		"ai_summary":     riskResult.AISummary,
		"message":        "Transaction received successfully",
	})
}

// this function  job is to receive a JSON object, check if it's valid, and send a response back.

func RegisterRoutes(router *gin.Engine) {

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "asguard health running",
		})
	})

	//What it does: It tells Gin,
	// "Before you run ANY route in this entire application,
	// run the APIKeyAuth function first."
	//The Result: Even your /health check now requires an API key
	//  in the header. If someone visits /health without the key,
	// they get a 401 Unauthorized.

	protected := router.Group("/")
	//router.Use(middleware.APIKeyAuth()). redundant code
	// A "Group" is used to organize routes that share the same prefix or logic.
	// By using "/", you aren't changing the URL (it's still just localhost:8081/),
	// but you are creating a logical "bucket" for your protected routes.

	protected.Use(middleware.APIKeyAuth())
	{
		protected.POST("/analyze", AnalyzeTransaction)
		protected.GET("/secure-test", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "API key valid"})
		})
	}
}
