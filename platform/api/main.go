package main

import (
	"log"
	"net/http"
	"os"
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", GetHealth)
	mux.HandleFunc("GET /api/fleet/stats", GetFleetStats)
	mux.HandleFunc("GET /api/fleet/alerts", GetFleetAlerts)
	mux.HandleFunc("GET /api/elevators", GetElevators)
	mux.HandleFunc("GET /api/elevators/{id}", GetElevatorByID)
	mux.HandleFunc("GET /api/elevators/{id}/inspections", GetElevatorInspections)
	mux.HandleFunc("GET /api/elevators/{id}/risk", GetElevatorRisk)

	log.Printf("server running on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
