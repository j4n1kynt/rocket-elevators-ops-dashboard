package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	if err := LoadFleetCSV("platform/elevator_fleet.csv"); err != nil {
		log.Fatalf("failed to load fleet data: %v", err)
	}
	log.Printf("loaded %d elevators", len(elevators))

	if err := LoadInspectionCSV("data/inspection.csv"); err != nil {
		log.Fatalf("failed to load inspection data: %v", err)
	}
	log.Printf("loaded inspection records for %d elevators", len(inspectionIdx))

	LoadPredictionsCSV("data/predictions.csv")
	if predictionsAvailable {
		EnrichElevatorsWithRisk()
		log.Printf("predictions loaded: /risk endpoint active")
	} else {
		log.Printf("predictions.csv not found: /risk returns 503")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/elevators", GetElevators)
	mux.HandleFunc("GET /api/elevators/{id}", GetElevatorByID)
	mux.HandleFunc("GET /api/elevators/{id}/inspections", GetElevatorInspections)
	mux.HandleFunc("GET /api/elevators/{id}/risk", GetElevatorRisk)

	log.Printf("server running on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
