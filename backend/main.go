package main

import (
	"asguard/routes" // importing the /routes package we created
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	router := gin.Default()

	// registers all routes
	routes.RegisterRoutes(router)

	router.Run(":8081")
}
