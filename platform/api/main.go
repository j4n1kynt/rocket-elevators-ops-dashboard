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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "mistral:7b"
	}
	log.Printf("chat using ollama at %s with model %s", ollamaURL, ollamaModel)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", GetHealth)
	mux.HandleFunc("GET /api/fleet/stats", GetFleetStats)
	mux.HandleFunc("GET /api/fleet/alerts", GetFleetAlerts)
	mux.HandleFunc("GET /api/elevators", GetElevators)
	mux.HandleFunc("GET /api/elevators/{id}", GetElevatorByID)
	mux.HandleFunc("GET /api/elevators/{id}/inspections", GetElevatorInspections)
	mux.HandleFunc("GET /api/elevators/{id}/risk", GetElevatorRisk)
	mux.HandleFunc("POST /api/chat", NewChatHandler(ollamaURL, ollamaModel))

	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
		// WriteTimeout must exceed the Ollama call budget, otherwise the server
		// cuts off a slow LLM reply before the chat handler can finish.
		WriteTimeout: ollamaRequestTimeout + 15*time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("server running on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
