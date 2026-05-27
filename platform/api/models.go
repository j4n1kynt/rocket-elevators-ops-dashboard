package main

type Elevator struct {
	ElevatorID              string  `json:"elevator_id"`
	Location                string  `json:"location"`
	LicenseNumber           string  `json:"license_number"`
	Status                  string  `json:"status"`
	ElevatorType            *string `json:"elevator_type"`
	LicenseExpirationDate   string  `json:"license_expiration_date"`
	LatestInspectionDate    *string `json:"latest_inspection_date"`
	LatestInspectionOutcome *string `json:"latest_inspection_outcome"`
}

type ElevatorListResponse struct {
	Total   int        `json:"total"`
	Page    int        `json:"page"`
	Limit   int        `json:"limit"`
	Results []Elevator `json:"results"`
}

type Inspection struct {
	InspectionNumber int    `json:"inspection_number"`
	InspectionType   string `json:"inspection_type"`
	InspectionDate   string `json:"inspection_date"`
	Outcome          string `json:"outcome"`
}

type ElevatorInspectionsResponse struct {
	ElevatorID  string       `json:"elevator_id"`
	Total       int          `json:"total"`
	Page        int          `json:"page"`
	Limit       int          `json:"limit"`
	Inspections []Inspection `json:"inspections"`
}

type RiskResponse struct {
	ElevatorID           string  `json:"elevator_id"`
	RiskScore            float64 `json:"risk_score"`
	RiskLevel            string  `json:"risk_level"`
	PredictedFailureDate *string `json:"predicted_failure_date"`
	Confidence           float64 `json:"confidence"`
	ModelVersion         string  `json:"model_version"`
	GeneratedAt          string  `json:"generated_at"`
}

type ErrorResponse struct {
	Error      string `json:"error"`
	ElevatorID string `json:"elevator_id,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
}