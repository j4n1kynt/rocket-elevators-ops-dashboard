package main

import (
	"encoding/csv"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	elevators            []Elevator
	elevatorIdx          map[string]*Elevator
	inspectionIdx        map[int][]Inspection
	riskIdx              map[string]*RiskResponse
	predictionsAvailable bool
)

func LoadFleetCSV(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return err
	}

	elevators = make([]Elevator, 0, len(rows)-1)

	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 8 {
			continue
		}
		elevators = append(elevators, Elevator{
			ElevatorID:              row[0],
			Location:                row[1],
			LicenseNumber:           row[2],
			Status:                  row[3],
			LicenseExpirationDate:   row[4],
			LatestInspectionDate:    nullableString(row[5]),
			LatestInspectionOutcome: nullableString(row[6]),
			ElevatorType:            nullableString(row[7]),
		})
	}

	// Build index after slice is fully populated to avoid pointer invalidation.
	elevatorIdx = make(map[string]*Elevator, len(elevators))
	for i := range elevators {
		elevatorIdx[elevators[i].ElevatorID] = &elevators[i]
	}

	return nil
}

func LoadInspectionCSV(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return err
	}

	inspectionIdx = make(map[int][]Inspection)

	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 9 {
			continue
		}

		devNum, err := strconv.Atoi(row[2])
		if err != nil {
			continue
		}
		inspNum, err := strconv.Atoi(row[3])
		if err != nil {
			continue
		}
		date, err := normalizeDate(row[7])
		if err != nil {
			log.Printf("skip inspection row %d: bad date %q: %v", i, row[7], err)
			continue
		}

		inspectionIdx[devNum] = append(inspectionIdx[devNum], Inspection{
			InspectionNumber: inspNum,
			InspectionType:   row[5],
			InspectionDate:   date,
			Outcome:          row[8],
		})
	}

	return nil
}

// LoadPredictionsCSV soft-fails if the file does not exist.
// Until predictions.csv is available, /risk returns 503.
// CSV columns (positional): elevator_id, risk_score, risk_level, model_version, prediction_date
func LoadPredictionsCSV(path string) {
	f, err := os.Open(path)
	if err != nil {
		predictionsAvailable = false
		return
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		predictionsAvailable = false
		return
	}

	riskIdx = make(map[string]*RiskResponse)

	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 5 {
			continue
		}
		riskScore, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue
		}
		// Confidence = max(riskScore, 1-riskScore): model certainty in its prediction direction.
		confidence := riskScore
		if confidence < 0.5 {
			confidence = 1 - confidence
		}
		r := &RiskResponse{
			ElevatorID:           row[0],
			RiskScore:            riskScore,
			RiskLevel:            row[2],
			PredictedFailureDate: nil,
			Confidence:           confidence,
			ModelVersion:         row[3],
			GeneratedAt:          row[4],
		}
		riskIdx[r.ElevatorID] = r
	}

	predictionsAvailable = true
}

// normalizeDate converts M/D/YYYY (inspection.csv raw format) to YYYY-MM-DD.
func normalizeDate(raw string) (string, error) {
	t, err := time.Parse("1/2/2006", raw)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}

func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// EnrichElevatorsWithRisk sets RiskLevel on each Elevator from the loaded
// predictions index. Call once after LoadPredictionsCSV succeeds.
func EnrichElevatorsWithRisk() {
	for i := range elevators {
		if risk, ok := riskIdx[elevators[i].ElevatorID]; ok {
			level := risk.RiskLevel
			elevators[i].RiskLevel = &level
		}
	}
}
