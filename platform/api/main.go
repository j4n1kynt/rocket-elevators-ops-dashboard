package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := InitDB(); err != nil {
		log.Fatalf("database unavailable: %v", err)
	}
	log.Printf("database connection established")

	// Warm the Ollama model in the background so the first chat message is fast.
	go WarmUpOllama()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", GetHealth)
	mux.HandleFunc("GET /api/fleet/stats", GetFleetStats)
	mux.HandleFunc("GET /api/fleet/alerts", GetFleetAlerts)
	mux.HandleFunc("GET /api/elevators", GetElevators)
	mux.HandleFunc("GET /api/elevators/{id}", GetElevatorByID)
	mux.HandleFunc("GET /api/elevators/{id}/inspections", GetElevatorInspections)
	mux.HandleFunc("GET /api/elevators/{id}/risk", GetElevatorRisk)
	mux.HandleFunc("POST /api/chat", PostChat)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 330 * time.Second, // extended for Ollama cold start (>300s for mistral:7b, EVAL-1)
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("server running on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
