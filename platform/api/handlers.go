package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func GetHealth(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(r.Context()); err != nil {
		writeJSON(w, 503, HealthResponse{Status: "degraded", Database: "unreachable"})
		return
	}
	writeJSON(w, 200, HealthResponse{Status: "ok", Database: "connected"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func GetElevators(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	statusFilter := q.Get("status")
	typeFilter   := q.Get("elevator_type")
	search       := q.Get("q")
	sortBy       := q.Get("sort")
	order        := strings.ToLower(q.Get("order"))

	page  := 1
	limit := 50

	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	if sortBy == "" {
		sortBy = "license_expiration_date"
	}
	if order == "" {
		order = "asc"
	}

	if sortBy != "license_expiration_date" && sortBy != "latest_inspection_date" {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid sort column. Accepted: license_expiration_date, latest_inspection_date"})
		return
	}
	if statusFilter != "" && statusFilter != "ACTIVE" && statusFilter != "BY REQUEST" {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid status value. Accepted: ACTIVE, BY REQUEST"})
		return
	}
	if limit < 1 {
		writeJSON(w, 400, ErrorResponse{Error: "limit must be at least 1"})
		return
	}
	if limit > 200 {
		writeJSON(w, 400, ErrorResponse{Error: "limit must not exceed 200"})
		return
	}

	// Validated sort column — safe to interpolate (not user input)
	sortCol := "e.license_expiry_date"
	if sortBy == "latest_inspection_date" {
		sortCol = "li.latest_inspection_date"
	}
	sortDir := "ASC"
	if order == "desc" {
		sortDir = "DESC"
	}

	// Build WHERE clause — condition strings are hardcoded; only values are args
	var conds []string
	var args  []any

	if statusFilter != "" {
		args = append(args, statusFilter)
		conds = append(conds, fmt.Sprintf("e.status = $%d", len(args)))
	}
	if typeFilter != "" {
		args = append(args, strings.ToLower(typeFilter))
		conds = append(conds, fmt.Sprintf("LOWER(e.elevator_type) = $%d", len(args)))
	}
	if search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf("(LOWER(e.elevator_id::text) LIKE $%d OR LOWER(e.location) LIKE $%d)", n, n))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// Count total matching rows (WHERE only references elevators — no JOIN needed)
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM elevators e %s", where)
	var total int
	if err := db.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	results := []Elevator{}
	offset := (page - 1) * limit

	if offset < total {
		// Append LIMIT and OFFSET as the next args
		args = append(args, limit)
		limitArg := len(args)
		args = append(args, offset)
		offsetArg := len(args)

		querySQL := fmt.Sprintf(`
			WITH li AS (
				SELECT DISTINCT ON (elevator_id)
					elevator_id, latest_inspection_date, outcome
				FROM inspections
				ORDER BY elevator_id, latest_inspection_date DESC NULLS LAST
			)
			SELECT
				e.elevator_id::text,
				e.location,
				e.license_number,
				e.status,
				e.elevator_type,
				e.license_expiry_date,
				li.latest_inspection_date,
				li.outcome,
				p.risk_level
			FROM elevators e
			LEFT JOIN li ON li.elevator_id = e.elevator_id
			LEFT JOIN predictions p ON p.elevator_id = e.elevator_id
			%s
			ORDER BY %s %s NULLS LAST
			LIMIT $%d OFFSET $%d`,
			where, sortCol, sortDir, limitArg, offsetArg,
		)

		rows, err := db.Query(r.Context(), querySQL, args...)
		if err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				elevatorID  string
				location    string
				licenseNum  string
				status      string
				elevType    *string
				licExpiry   *time.Time
				inspDate    *time.Time
				inspOutcome *string
				riskLevel   *string
			)
			if err := rows.Scan(
				&elevatorID, &location, &licenseNum, &status,
				&elevType, &licExpiry, &inspDate, &inspOutcome, &riskLevel,
			); err != nil {
				writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
				return
			}

			licExp := ""
			if licExpiry != nil {
				licExp = licExpiry.Format("2006-01-02")
			}

			results = append(results, Elevator{
				ElevatorID:              elevatorID,
				Location:                location,
				LicenseNumber:           licenseNum,
				Status:                  status,
				ElevatorType:            elevType,
				LicenseExpirationDate:   licExp,
				LatestInspectionDate:    formatDate(inspDate),
				LatestInspectionOutcome: inspOutcome,
				RiskLevel:               riskLevel,
			})
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
			return
		}
	}

	writeJSON(w, 200, ElevatorListResponse{
		Total:   total,
		Page:    page,
		Limit:   limit,
		Results: results,
	})
}

func GetElevatorByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if !isNumeric(id) {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid elevator ID format. ID must be numeric."})
		return
	}

	elevID, _ := strconv.Atoi(id)

	var (
		elevatorID  string
		location    string
		licenseNum  string
		status      string
		elevType    *string
		licExpiry   *time.Time
		inspDate    *time.Time
		inspOutcome *string
		riskLevel   *string
	)

	err := db.QueryRow(r.Context(), `
		SELECT
			e.elevator_id::text,
			e.location,
			e.license_number,
			e.status,
			e.elevator_type,
			e.license_expiry_date,
			li.latest_inspection_date,
			li.outcome,
			p.risk_level
		FROM elevators e
		LEFT JOIN LATERAL (
			SELECT latest_inspection_date, outcome
			FROM inspections
			WHERE elevator_id = e.elevator_id
			ORDER BY latest_inspection_date DESC NULLS LAST
			LIMIT 1
		) li ON true
		LEFT JOIN predictions p ON p.elevator_id = e.elevator_id
		WHERE e.elevator_id = $1`,
		elevID,
	).Scan(
		&elevatorID, &location, &licenseNum, &status,
		&elevType, &licExpiry, &inspDate, &inspOutcome, &riskLevel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	licExp := ""
	if licExpiry != nil {
		licExp = licExpiry.Format("2006-01-02")
	}

	writeJSON(w, 200, Elevator{
		ElevatorID:              elevatorID,
		Location:                location,
		LicenseNumber:           licenseNum,
		Status:                  status,
		ElevatorType:            elevType,
		LicenseExpirationDate:   licExp,
		LatestInspectionDate:    formatDate(inspDate),
		LatestInspectionOutcome: inspOutcome,
		RiskLevel:               riskLevel,
	})
}

func GetElevatorInspections(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if !isNumeric(id) {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid elevator ID format. ID must be numeric."})
		return
	}

	elevID, _ := strconv.Atoi(id)

	q := r.URL.Query()
	page  := 1
	limit := 50

	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		writeJSON(w, 400, ErrorResponse{Error: "limit must be at least 1"})
		return
	}
	if limit > 200 {
		writeJSON(w, 400, ErrorResponse{Error: "limit must not exceed 200"})
		return
	}

	// Check elevator exists
	var exists bool
	if err := db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = $1)", elevID,
	).Scan(&exists); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	if !exists {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}

	var total int
	if err := db.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM inspections WHERE elevator_id = $1", elevID,
	).Scan(&total); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	records := []Inspection{}
	offset := (page - 1) * limit

	if offset < total {
		rows, err := db.Query(r.Context(), `
			SELECT
				inspection_id,
				COALESCE(inspection_type, '') AS inspection_type,
				latest_inspection_date,
				outcome
			FROM inspections
			WHERE elevator_id = $1
			ORDER BY latest_inspection_date DESC NULLS LAST
			LIMIT $2 OFFSET $3`,
			elevID, limit, offset,
		)
		if err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				inspNum  int
				inspType string
				inspDate *time.Time
				outcome  string
			)
			if err := rows.Scan(&inspNum, &inspType, &inspDate, &outcome); err != nil {
				writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
				return
			}

			dateStr := ""
			if inspDate != nil {
				dateStr = inspDate.Format("2006-01-02")
			}

			records = append(records, Inspection{
				InspectionNumber: inspNum,
				InspectionType:   inspType,
				InspectionDate:   dateStr,
				Outcome:          outcome,
			})
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
			return
		}
	}

	writeJSON(w, 200, ElevatorInspectionsResponse{
		ElevatorID:  id,
		Total:       total,
		Page:        page,
		Limit:       limit,
		Inspections: records,
	})
}

func GetElevatorRisk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if !isNumeric(id) {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid elevator ID format. ID must be numeric."})
		return
	}

	elevID, _ := strconv.Atoi(id)

	// 404 on unknown elevator checked before prediction lookup, per spec
	var exists bool
	if err := db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM elevators WHERE elevator_id = $1)", elevID,
	).Scan(&exists); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	if !exists {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}

	var (
		elevIDStr       string
		riskScore       float64
		riskLevel       string
		riskExplanation *string
		modelVersion    string
		predDate        time.Time
	)

	err := db.QueryRow(r.Context(), `
		SELECT elevator_id::text, risk_score::float8, risk_level,
		       risk_explanation, model_version, prediction_date
		FROM predictions
		WHERE elevator_id = $1`,
		elevID,
	).Scan(&elevIDStr, &riskScore, &riskLevel, &riskExplanation, &modelVersion, &predDate)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 404, ErrorResponse{Error: "No prediction available for this elevator.", ElevatorID: id})
		return
	}
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	confidence := riskScore
	if confidence < 0.5 {
		confidence = 1 - confidence
	}

	writeJSON(w, 200, RiskResponse{
		ElevatorID:           elevIDStr,
		RiskScore:            riskScore,
		RiskLevel:            riskLevel,
		PredictedFailureDate: nil,
		Confidence:           confidence,
		ModelVersion:         modelVersion,
		GeneratedAt:          predDate.Format("2006-01-02"),
		RiskExplanation:      riskExplanation,
	})
}

func GetFleetStats(w http.ResponseWriter, r *http.Request) {
	// Risk distribution + total (one query)
	var (
		totalElevators int
		lowCount       int
		mediumCount    int
		highCount      int
		unknownCount   int
	)
	err := db.QueryRow(r.Context(), `
		SELECT
			COUNT(*) AS total,
			COUNT(CASE WHEN LOWER(p.risk_level) = 'low'     THEN 1 END) AS low,
			COUNT(CASE WHEN LOWER(p.risk_level) = 'medium'  THEN 1 END) AS medium,
			COUNT(CASE WHEN LOWER(p.risk_level) = 'high'    THEN 1 END) AS high,
			COUNT(CASE WHEN p.risk_level IS NULL OR LOWER(p.risk_level) NOT IN ('low', 'medium', 'high') THEN 1 END) AS unknown
		FROM elevators e
		LEFT JOIN predictions p ON p.elevator_id = e.elevator_id`,
	).Scan(&totalElevators, &lowCount, &mediumCount, &highCount, &unknownCount)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	// Elevators with at least one passing inspection
	var passingCount int
	err = db.QueryRow(r.Context(), `
		SELECT COUNT(DISTINCT elevator_id)
		FROM inspections
		WHERE LOWER(outcome) IN ('passed', 'all orders resolved')`,
	).Scan(&passingCount)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	passRate := 0.0
	if totalElevators > 0 {
		passRate = float64(passingCount) / float64(totalElevators)
	}

	// Equipment type distribution
	typeRows, err := db.Query(r.Context(), `
		SELECT COALESCE(elevator_type, 'null') AS type, COUNT(*) AS cnt
		FROM elevators
		GROUP BY elevator_type`)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	defer typeRows.Close()

	typeCounts := make(map[string]int)
	for typeRows.Next() {
		var typeName string
		var cnt int
		if err := typeRows.Scan(&typeName, &cnt); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
			return
		}
		typeCounts[typeName] = cnt
	}
	if err := typeRows.Err(); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
		return
	}

	writeJSON(w, 200, FleetStatsResponse{
		TotalElevators: totalElevators,
		RiskDistribution: RiskDistribution{
			Low:     lowCount,
			Medium:  mediumCount,
			High:    highCount,
			Unknown: unknownCount,
		},
		InspectionPassRate:        passRate,
		EquipmentTypeDistribution: typeCounts,
	})
}

func GetFleetAlerts(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(), `
		WITH li AS (
			SELECT DISTINCT ON (elevator_id)
				elevator_id, latest_inspection_date, outcome
			FROM inspections
			ORDER BY elevator_id, latest_inspection_date DESC NULLS LAST
		)
		SELECT
			e.elevator_id::text,
			e.location,
			p.risk_level,
			p.risk_score::float8,
			li.latest_inspection_date,
			li.outcome
		FROM elevators e
		JOIN predictions p ON p.elevator_id = e.elevator_id
		LEFT JOIN li ON li.elevator_id = e.elevator_id
		WHERE p.risk_level = 'HIGH'
		  AND (
			li.elevator_id IS NULL
			OR LOWER(li.outcome) NOT IN ('passed', 'all orders resolved')
		  )
		ORDER BY p.risk_score DESC`)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	defer rows.Close()

	alerts := []AlertEntry{}

	for rows.Next() {
		var (
			elevatorID  string
			location    string
			riskLevel   string
			riskScore   float64
			inspDate    *time.Time
			inspOutcome *string
		)
		if err := rows.Scan(
			&elevatorID, &location, &riskLevel, &riskScore,
			&inspDate, &inspOutcome,
		); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
			return
		}

		confidence := riskScore
		if confidence < 0.5 {
			confidence = 1 - confidence
		}

		alerts = append(alerts, AlertEntry{
			ElevatorID:              elevatorID,
			Location:                location,
			RiskLevel:               riskLevel,
			RiskScore:               riskScore,
			Confidence:              confidence,
			LatestInspectionDate:    formatDate(inspDate),
			LatestInspectionOutcome: inspOutcome,
		})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
		return
	}

	writeJSON(w, 200, FleetAlertsResponse{Total: len(alerts), Alerts: alerts})
}
