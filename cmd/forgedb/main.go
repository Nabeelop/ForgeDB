package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"forgedb/internal/server"
	"forgedb/internal/storage"
)

func main() {
	dbPath := flag.String("db", "data.db", "Path to database file")
	addr := flag.String("addr", ":9090", "TCP address to listen on")
	flag.Parse()

	log.Printf("Starting ForgeDB server...")
	log.Printf("Database file: %s", *dbPath)

	db, err := storage.Open(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		log.Println("Closing database...")
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	log.Printf("Starting TCP listener on %s", *addr)
	srv, err := server.NewServer(*addr, db)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Channel to capture termination signals for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("Server listener stopped: %v", err)
		}
	}()

	log.Println("ForgeDB is ready for connections. Press Ctrl+C to stop.")

	// Wait for termination signal
	<-sigChan
	log.Println("Shutting down server...")

	if err := srv.Close(); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}
}
