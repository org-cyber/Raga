package main

import (
	"asguard/routes" // importing the /routes package we created
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		//log.Fatalf("Error loading .env file: %v", err)
		log.Printf("env variables werent found, using system vars")
	}

	router := gin.Default()

	// registers all routes
	routes.RegisterRoutes(router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if port[0] != ':' {
		port = ":" + port
	}

	router.Run(port)
}
