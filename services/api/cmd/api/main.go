package main

import (
	"log"

	"central-api/internal/database"
	"central-api/internal/server"
)

func main() {
	log.Println("Starting Central Control Plane API...")

	// Initialize Database (Reads DB_DSN from environment)
	if err := database.InitDB(); err != nil {
		log.Printf("Warning: Database initialization failed (using mock mode): %v", err)
	}

	srv := server.NewServer()
	
	if err := srv.Start(":9090"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
