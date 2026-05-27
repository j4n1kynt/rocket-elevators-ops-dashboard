package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func GetElevators(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	statusFilter := q.Get("status")
	typeFilter := q.Get("elevator_type")
	search := q.Get("q")
	sortBy := q.Get("sort")
	order := q.Get("order")

	page := 1
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
	if limit > 200 {
		writeJSON(w, 400, ErrorResponse{Error: "limit must not exceed 200"})
		return
	}

	results := make([]Elevator, 0, len(elevators))
	for _, e := range elevators {
		if statusFilter != "" && e.Status != statusFilter {
			continue
		}
		if typeFilter != "" {
			if e.ElevatorType == nil || !strings.EqualFold(*e.ElevatorType, typeFilter) {
				continue
			}
		}
		if search != "" {
			ql := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(e.ElevatorID), ql) &&
				!strings.Contains(strings.ToLower(e.Location), ql) {
				continue
			}
		}
		results = append(results, e)
	}

	sort.Slice(results, func(i, j int) bool {
		var a, b *string
		if sortBy == "latest_inspection_date" {
			a = results[i].LatestInspectionDate
			b = results[j].LatestInspectionDate
		} else {
			ai := results[i].LicenseExpirationDate
			bi := results[j].LicenseExpirationDate
			a, b = &ai, &bi
		}
		// Nulls always last, regardless of sort direction.
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		if order == "desc" {
			return *a > *b
		}
		return *a < *b
	})

	total := len(results)
	offset := (page - 1) * limit
	if offset >= total {
		results = []Elevator{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		results = results[offset:end]
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

	e, ok := elevatorIdx[id]
	if !ok {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}

	writeJSON(w, 200, *e)
}

func GetElevatorInspections(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if !isNumeric(id) {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid elevator ID format. ID must be numeric."})
		return
	}

	if _, ok := elevatorIdx[id]; !ok {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}

	q := r.URL.Query()
	page := 1
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

	if limit > 200 {
		writeJSON(w, 400, ErrorResponse{Error: "limit must not exceed 200"})
		return
	}

	devNum, _ := strconv.Atoi(id)
	src := inspectionIdx[devNum]
	records := make([]Inspection, len(src))
	copy(records, src)

	// YYYY-MM-DD strings sort lexicographically == chronologically.
	sort.Slice(records, func(i, j int) bool {
		return records[i].InspectionDate > records[j].InspectionDate
	})

	total := len(records)
	offset := (page - 1) * limit
	if offset >= total {
		records = []Inspection{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		records = records[offset:end]
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

	if !predictionsAvailable {
		writeJSON(w, 503, ErrorResponse{
			Error:    "Risk data unavailable. Predictions pipeline not yet deployed.",
			Endpoint: "/api/elevators/" + id + "/risk",
		})
		return
	}

	if _, ok := elevatorIdx[id]; !ok {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}

	risk, ok := riskIdx[id]
	if !ok {
		writeJSON(w, 404, ErrorResponse{Error: "Elevator not found.", ElevatorID: id})
		return
	}

	writeJSON(w, 200, *risk)
}
