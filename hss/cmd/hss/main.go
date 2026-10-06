package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"hss/internal/api"
	"hss/internal/camera"
	"hss/internal/config"
	"hss/internal/control"
	"hss/internal/database"
	"hss/internal/events"
	"hss/internal/recording"
	"hss/internal/security"
)

func main() {
	log.Println("Starting Home Surveillance Server (HSS)...")

	// 1. Initialize Configuration
	config.InitConfig()
	log.Println("Configuration loaded successfully.")

	// 2. Setup Context for Graceful Shutdown
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Catch SIGTERM and SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v. Initiating graceful shutdown...\n", sig)
		cancel() // This cascades to all workers
	}()

	// 3. Initialize SQLite Database
	if err := database.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 4. Initialize Cryptographic Identity
	if err := security.InitIdentity(); err != nil {
		log.Fatalf("Failed to initialize identity: %v", err)
	}
	log.Printf("HSS Core Supervisor running. Device ID: %s\n", security.DeviceID)

	// 5. Initialize Camera Manager
	if err := camera.InitManager(ctx); err != nil {
		log.Fatalf("Failed to initialize camera manager: %v", err)
	}

	// 6. Start Global Services
	
	// Start Event Engine
	events.InitEngine(ctx)

	// Start Retention Engine
	retentionEngine := recording.NewRetentionEngine()
	wg.Add(1)
	go func() {
		defer wg.Done()
		retentionEngine.Start(ctx)
	}()

	// Start Control Plane Client
	// MVP: Use localhost for testing. In prod, use config.AppConfig.Control.Endpoint
	controlClient := control.NewClient("ws://localhost:9090/ws/device")
	wg.Add(1)
	go func() {
		defer wg.Done()
		go control.StartPairingRoutine(ctx)
		go controlClient.Connect()
		<-ctx.Done()
		log.Println("Shutting down Control Client...")
	}()

	// 7. Start Local API Server
	apiServer := api.NewServer(config.AppConfig.API.Port)
	wg.Add(1)
	go func() {
		defer wg.Done()
		apiServer.Start(ctx)
	}()

	// 8. Wait for all goroutines to gracefully exit upon cancellation
	<-ctx.Done()
	log.Println("Waiting for all background services to finish...")
	wg.Wait()
	
	log.Println("HSS shutdown complete.")
}
