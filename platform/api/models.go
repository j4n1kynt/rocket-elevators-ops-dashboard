package main

import "context"

type Elevator struct {
	ElevatorID              string  `json:"elevator_id"`
	Location                string  `json:"location"`
	LicenseNumber           string  `json:"license_number"`
	Status                  string  `json:"status"`
	ElevatorType            *string `json:"elevator_type"`
	LicenseExpirationDate   string  `json:"license_expiration_date"`
	LatestInspectionDate    *string `json:"latest_inspection_date"`
	LatestInspectionOutcome *string `json:"latest_inspection_outcome"`
	RiskLevel               *string `json:"risk_level"`
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
	RiskExplanation      *string `json:"risk_explanation"`
}

type ErrorResponse struct {
	Error      string `json:"error"`
	ElevatorID string `json:"elevator_id,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
}

type RiskDistribution struct {
	Low     int `json:"low"`
	Medium  int `json:"medium"`
	High    int `json:"high"`
	Unknown int `json:"unknown"`
}

type FleetStatsResponse struct {
	TotalElevators            int              `json:"total_elevators"`
	RiskDistribution          RiskDistribution `json:"risk_distribution"`
	InspectionPassRate        float64          `json:"inspection_pass_rate"`
	EquipmentTypeDistribution map[string]int   `json:"equipment_type_distribution"`
}

type AlertEntry struct {
	ElevatorID              string  `json:"elevator_id"`
	Location                string  `json:"location"`
	RiskLevel               string  `json:"risk_level"`
	RiskScore               float64 `json:"risk_score"`
	Confidence              float64 `json:"confidence"`
	LatestInspectionDate    *string `json:"latest_inspection_date"`
	LatestInspectionOutcome *string `json:"latest_inspection_outcome"`
}

type FleetAlertsResponse struct {
	Total  int          `json:"total"`
	Page   int          `json:"page"`
	Limit  int          `json:"limit"`
	Alerts []AlertEntry `json:"alerts"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PendingAction holds the validated Phase 1 scheduling state that must survive
// between the confirmation prompt turn and the user's yes/no reply (spec §7.2).
// Stored client-side in a hidden form field and echoed on every request.
//
// Signature is an HMAC over the execution fields (elevator_id, date, type,
// reason), computed by the server in Phase 1 and re-checked in Phase 2. Because
// the client holds and replays this object, the signature is what makes Phase 1
// a real gate: Phase 2 only writes values that a genuine Phase 1 preview on this
// server produced — a forged or tampered pending_action is rejected before any
// database write.
type PendingAction struct {
	ElevatorID     int    `json:"elevator_id"`
	InspectionDate string `json:"inspection_date"`
	InspectionType string `json:"inspection_type"`
	Reason         string `json:"reason"`
	Summary        string `json:"summary"`
	ExpiresAt      int64  `json:"expires_at,omitempty"` // Unix seconds; signed and checked in Phase 2
	Signature      string `json:"signature,omitempty"`
}

type ChatRequest struct {
	Message       string         `json:"message"`
	History       []ChatMessage  `json:"history"`
	PendingAction *PendingAction `json:"pending_action,omitempty"`
	// ConversationID links turns into one conversation. The server sets it on
	// the first turn; the client sends it back on every following turn.
	ConversationID int64 `json:"conversation_id,omitempty"`
}

type ChatResponse struct {
	Reply          string         `json:"reply"`
	History        []ChatMessage  `json:"history"`
	PendingAction  *PendingAction `json:"pending_action,omitempty"`
	ConversationID int64          `json:"conversation_id,omitempty"`
}

// AgentFunc is the common callable contract for all agents.
type AgentFunc func(ctx context.Context, req AgentRequest) AgentResponse

type AgentRequest struct {
	Message       string
	History       []ChatMessage
	PendingAction *PendingAction
	AllowedTools  []string
}

type AgentResponse struct {
	Reply          string
	AgentName      string
	PendingAction  *PendingAction
	UpdatedHistory []ChatMessage
}
