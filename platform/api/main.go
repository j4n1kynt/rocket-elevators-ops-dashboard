package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Load .env for local `go run` so DATABASE_URL and friends are available.
	// Real environment variables (e.g. from docker-compose) are never overridden.
	if err := loadDotEnv(".env"); err != nil {
		log.Printf("warning: could not read .env: %v", err)
	}

	if err := InitDB(); err != nil {
		log.Fatalf("database unavailable: %v", err)
	}
	log.Printf("database connection established")

	go WarmUpLLM()

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
	mux.Handle("GET /api/conversations", route(GetConversations))
	mux.Handle("GET /api/conversations/stats", route(GetConversationStats))
	mux.Handle("GET /api/conversations/{id}", route(GetConversationByID))
	mux.HandleFunc("POST /api/chat", PostChat) // manages its own 330s deadline via context

	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
		// WriteTimeout is 0 (disabled) — /api/chat blocks up to 330s for the LLM;
		// all other routes enforce their own deadline via http.TimeoutHandler above.
		IdleTimeout: 60 * time.Second,
	}
	log.Printf("server running on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
