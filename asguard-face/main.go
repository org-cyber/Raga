package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Kagami/go-face"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Global recognizer - loaded once, used by all requests
var recognizer *face.Recognizer

// API keys loaded from environment
var apiKeys map[string]bool

func main() {
	// Get configuration from environment
	modelsPath := getEnv("MODELS_PATH", "./models")
	apiKeyList := getEnv("API_KEYS", "dev-key-123")

	// Parse API keys (comma-separated)
	apiKeys = make(map[string]bool)
	for _, key := range strings.Split(apiKeyList, ",") {
		apiKeys[strings.TrimSpace(key)] = true
	}

	// Initialize face recognizer (loads dlib models)
	var err error
	recognizer, err = face.NewRecognizer(modelsPath)
	if err != nil {
		log.Fatalf("Failed to load face models from %s: %v", modelsPath, err)
	}
	defer recognizer.Close()

	log.Printf("Loaded face models from %s", modelsPath)
	log.Printf("API keys configured: %d", len(apiKeys))

	// Set up HTTP server
	// Set up HTTP server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestIDMiddleware())
	// Enable CORS with custom config
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // Allow all origins (or specify your frontend URL)
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"}, // ADD Authorization
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))
	r.Use(authMiddleware())

	// Routes
	r.POST("/v1/analyze", handleAnalyze)
	r.POST("/v1/compare", handleCompare)
	r.GET("/health", handleHealth)

	port := getEnv("PORT", "8082")
	log.Printf("Starting Asguard Face Service on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// Request types
type AnalyzeRequest struct {
	Image         string `json:"image" binding:"required"`
	QualityChecks bool   `json:"quality_checks"`
}

type AnalyzeResponse struct {
	Success          bool      `json:"success"`
	FaceDetected     bool      `json:"face_detected"`
	Embedding        []float32 `json:"embedding,omitempty"`
	QualityScore     float32   `json:"quality_score,omitempty"`
	Sharpness        float32   `json:"sharpness,omitempty"`
	Brightness       float32   `json:"brightness,omitempty"`
	FaceSizeRatio    float32   `json:"face_size_ratio,omitempty"`
	Warnings         []string  `json:"warnings,omitempty"`
	ProcessingTimeMs int64     `json:"processing_time_ms"`
	Error            string    `json:"error,omitempty"`
}

type CompareRequest struct {
	ProbeImage         string    `json:"probe_image" binding:"required"`
	ReferenceEmbedding []float32 `json:"reference_embedding" binding:"required"`
	Threshold          *float32  `json:"threshold,omitempty"`
}

type CompareResponse struct {
	Success          bool    `json:"success"`
	Match            bool    `json:"match"`
	Confidence       float32 `json:"confidence"`
	Distance         float32 `json:"distance"`
	ThresholdUsed    float32 `json:"threshold_used"`
	ProbeQuality     float32 `json:"probe_quality,omitempty"`
	ProcessingTimeMs int64   `json:"processing_time_ms"`
	Error            string  `json:"error,omitempty"`
}

// Handler: Extract embedding from image
func handleAnalyze(c *gin.Context) {
	start := time.Now()
	requestID := c.GetString("request_id")

	var req AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AnalyzeResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	// Decode base64 to raw bytes
	imgBytes, err := decodeBase64ToBytes(req.Image)
	if err != nil {
		log.Printf("[%s] Image decode error: %v", requestID, err)
		c.JSON(http.StatusOK, AnalyzeResponse{
			Success: false,
			Error:   "Invalid image: " + err.Error(),
		})
		return
	}

	// Decode to image.Image for quality checks (if needed)
	var img image.Image
	if req.QualityChecks {
		img, _, err = image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			log.Printf("[%s] Image decode for quality failed: %v", requestID, err)
			// Continue without quality checks
			img = nil
		}
	}

	log.Printf("[%s] Decoded image, size %d bytes", requestID, len(imgBytes))

	// Recognize faces - pass raw bytes to go-face
	faces, err := recognizer.Recognize(imgBytes)
	if err != nil {
		log.Printf("[%s] Recognition error: %v", requestID, err)
		c.JSON(http.StatusOK, AnalyzeResponse{
			Success: false,
			Error:   "Face processing failed: " + err.Error(),
		})
		return
	}

	if len(faces) == 0 {
		c.JSON(http.StatusOK, AnalyzeResponse{
			Success:      false,
			FaceDetected: false,
			Error:        "No face detected",
		})
		return
	}

	if len(faces) > 1 {
		c.JSON(http.StatusOK, AnalyzeResponse{
			Success:      false,
			FaceDetected: true,
			Error:        fmt.Sprintf("Multiple faces detected: %d found, expected 1", len(faces)),
		})
		return
	}

	faceData := faces[0]
	embedding := faceData.Descriptor[:]

	// Quality checks
	var warnings []string
	var qualityScore, sharpness, brightness, faceSize float32 = 1.0, 1.0, 128, 0.3

	if req.QualityChecks && img != nil {
		qualityScore, sharpness, brightness, faceSize, warnings = checkQuality(img, faceData.Rectangle)
	}

	elapsed := time.Since(start).Milliseconds()

	c.JSON(http.StatusOK, AnalyzeResponse{
		Success:          true,
		FaceDetected:     true,
		Embedding:        embedding,
		QualityScore:     qualityScore,
		Sharpness:        sharpness,
		Brightness:       brightness,
		FaceSizeRatio:    faceSize,
		Warnings:         warnings,
		ProcessingTimeMs: elapsed,
	})
}

// Handler: Compare probe image with reference embedding
func handleCompare(c *gin.Context) {
	start := time.Now()

	var req CompareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CompareResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	// Validate embedding length
	if len(req.ReferenceEmbedding) != 128 {
		c.JSON(http.StatusOK, CompareResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid embedding: expected 128 dimensions, got %d", len(req.ReferenceEmbedding)),
		})
		return
	}

	// Decode probe image to bytes
	imgBytes, err := decodeBase64ToBytes(req.ProbeImage)
	if err != nil {
		c.JSON(http.StatusOK, CompareResponse{
			Success: false,
			Error:   "Invalid probe image: " + err.Error(),
		})
		return
	}

	// Decode to image.Image for quality checks
	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		img = nil // Continue without quality checks
	}

	// Extract embedding from probe - pass raw bytes
	faces, err := recognizer.Recognize(imgBytes)
	if err != nil {
		c.JSON(http.StatusOK, CompareResponse{
			Success: false,
			Error:   "Face processing failed: " + err.Error(),
		})
		return
	}

	if len(faces) == 0 {
		c.JSON(http.StatusOK, CompareResponse{
			Success: false,
			Error:   "No face detected in probe image",
		})
		return
	}

	if len(faces) > 1 {
		c.JSON(http.StatusOK, CompareResponse{
			Success: false,
			Error:   fmt.Sprintf("Multiple faces in probe image: %d found", len(faces)),
		})
		return
	}

	// Convert embeddings to arrays for comparison
	var refArray [128]float32
	var probeArray [128]float32
	copy(refArray[:], req.ReferenceEmbedding)
	probeArray = faces[0].Descriptor

	// Calculate distance
	threshold := float32(0.6)
	if req.Threshold != nil {
		threshold = *req.Threshold
	}

	match, confidence, distance := compareEmbeddings(refArray, probeArray, threshold)

	// Quality check on probe
	var probeQuality float32
	if img != nil {
		probeQuality, _, _, _, _ = checkQuality(img, faces[0].Rectangle)
	}

	elapsed := time.Since(start).Milliseconds()

	c.JSON(http.StatusOK, CompareResponse{
		Success:          true,
		Match:            match,
		Confidence:       confidence,
		Distance:         distance,
		ThresholdUsed:    threshold,
		ProbeQuality:     probeQuality,
		ProcessingTimeMs: elapsed,
	})
}

// Handler: Health check
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "asguard-face",
		"version": "1.0.0",
	})
}

// Compare two 128D embeddings using Euclidean distance
func compareEmbeddings(ref, probe [128]float32, threshold float32) (match bool, confidence, distance float32) {
	var sum float32
	for i := 0; i < 128; i++ {
		diff := ref[i] - probe[i]
		sum += diff * diff
	}

	// Use standard library math.Sqrt
	distance = float32(math.Sqrt(float64(sum)))

	// Convert to confidence: 0 distance = 1.0 confidence, threshold = 0 confidence
	if distance >= threshold {
		confidence = 0
	} else {
		confidence = 1 - (distance / threshold)
	}

	match = distance < threshold
	return
}

// Quality check: returns overall score and warnings
func checkQuality(img image.Image, faceRect image.Rectangle) (score, sharpness, brightness, faceSize float32, warnings []string) {
	bounds := img.Bounds()
	imgArea := bounds.Dx() * bounds.Dy()
	faceArea := (faceRect.Max.X - faceRect.Min.X) * (faceRect.Max.Y - faceRect.Min.Y)
	faceSize = float32(faceArea) / float32(imgArea)

	// Brightness check (average pixel value)
	var totalBrightness float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Convert to grayscale using standard weights
			gray := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
			totalBrightness += gray
		}
	}
	brightness = float32(totalBrightness / float64(imgArea))

	// Sharpness (Laplacian variance approximation)
	sharpness = calculateSharpness(img)

	// Check thresholds
	if faceSize < 0.15 {
		warnings = append(warnings, "face_too_small")
	}
	if brightness < 60 {
		warnings = append(warnings, "too_dark")
	} else if brightness > 200 {
		warnings = append(warnings, "too_bright")
	}
	if sharpness < 100 {
		warnings = append(warnings, "too_blurry")
	}

	// Overall score
	score = 1.0
	if len(warnings) > 0 {
		score = 0.7 // Penalize for warnings
	}
	return
}

// Calculate sharpness using Laplacian variance
func calculateSharpness(img image.Image) float32 {
	bounds := img.Bounds()
	if bounds.Dx() < 3 || bounds.Dy() < 3 {
		return 0
	}

	var sum, sumSq float64
	count := 0

	// Sample every 4th pixel for performance
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y += 4 {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x += 4 {
			// Simple gradient magnitude (Sobel approximation)
			gx := grayDiff(img, x+1, y, x-1, y)
			gy := grayDiff(img, x, y+1, x, y-1)
			mag := math.Sqrt(gx*gx + gy*gy)

			sum += mag
			sumSq += mag * mag
			count++
		}
	}

	if count == 0 {
		return 0
	}

	mean := sum / float64(count)
	variance := (sumSq / float64(count)) - (mean * mean)

	// Normalize: typical variance ranges 0-1000, cap at 1000
	normalized := variance / 1000.0
	if normalized > 1.0 {
		normalized = 1.0
	}
	return float32(normalized)
}

func grayDiff(img image.Image, x1, y1, x2, y2 int) float64 {
	// Get grayscale values at two points
	r1, g1, b1, _ := img.At(x1, y1).RGBA()
	r2, g2, b2, _ := img.At(x2, y2).RGBA()

	gray1 := 0.299*float64(r1>>8) + 0.587*float64(g1>>8) + 0.114*float64(b1>>8)
	gray2 := 0.299*float64(r2>>8) + 0.587*float64(g2>>8) + 0.114*float64(b2>>8)

	return gray1 - gray2
}

// Decode base64 string to raw bytes
func decodeBase64ToBytes(base64Str string) ([]byte, error) {
	// Remove data URI prefix if present
	if idx := strings.Index(base64Str, ","); idx != -1 {
		base64Str = base64Str[idx+1:]
	}

	data, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	return data, nil
}

// Middleware: Add request ID for tracing
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// Middleware: Check API key
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for health check
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		key := c.GetHeader("Authorization")
		// Remove "Bearer " prefix if present
		key = strings.TrimPrefix(key, "Bearer ")

		if !apiKeys[key] {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or missing API key",
			})
			return
		}
		c.Next()
	}
}

// Helper: Get environment variable with default
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
