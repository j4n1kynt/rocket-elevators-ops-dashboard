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

	route := func(h http.HandlerFunc) http.Handler {
		return http.TimeoutHandler(h, 15*time.Second, `{"error":"request timeout"}`)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /health", route(GetHealth))
	mux.Handle("GET /api/fleet/stats", route(GetFleetStats))
	mux.Handle("GET /api/fleet/alerts", route(GetFleetAlerts))
	mux.Handle("GET /api/elevators", route(GetElevators))
	mux.Handle("GET /api/elevators/{id}", route(GetElevatorByID))
	mux.Handle("GET /api/elevators/{id}/inspections", route(GetElevatorInspections))
	mux.Handle("GET /api/elevators/{id}/risk", route(GetElevatorRisk))
	mux.HandleFunc("POST /api/chat", PostChat) // manages its own 330s deadline via context

	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
		// WriteTimeout is 0 (disabled) — /api/chat blocks up to 330s for Ollama;
		// all other routes enforce their own deadline via http.TimeoutHandler above.
		IdleTimeout: 60 * time.Second,
	}
	log.Printf("server running on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
