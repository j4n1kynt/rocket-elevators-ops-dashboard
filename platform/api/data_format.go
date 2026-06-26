package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// formatToolResult turns a tool's JSON payload into a deterministic, plain-text
// data block (no markdown). The block is built in Go so the model never touches
// the numbers — it only writes a one-line intro on top (hybrid formatting, S3-3).
//
// It returns ok=false for any tool that does not yet have a Go formatter or for a
// payload it cannot parse, so the data agent falls back to the inject-and-answer
// path.
func formatToolResult(toolName, jsonText string) (string, bool) {
	switch toolName {
	case "get_elevator_risk":
		return formatRiskBlock(jsonText)
	case "get_fleet_stats":
		return formatFleetStats(jsonText)
	case "get_inspection_history":
		return formatInspectionHistory(jsonText)
	case "get_elevator_incidents":
		return formatElevatorIncidents(jsonText)
	case "get_elevators_needing_followup":
		return formatFollowup(jsonText)
	case "get_tssa_shutdown_elevators":
		return formatShutdown(jsonText)
	case "get_incident_count_last_year":
		return formatIncidentCount(jsonText)
	default:
		return "", false
	}
}

// ── Shared helpers ────────────────────────────────────────────────────────────

// sourceLine builds the standard "Source:" line with a clean, human label.
// The label is fixed per formatter, so wording stays consistent and never
// echoes the tool's raw internal source string.
func sourceLine(label string) string {
	return "Source: live fleet database — " + label
}

// notFoundBlock is the shared block for a lookup whose elevator does not exist.
func notFoundBlock(elevatorID int) string {
	return fmt.Sprintf("Source: live fleet database\n\nElevator %d was not found in the fleet database.", elevatorID)
}

// strOr returns the trimmed string value, or fallback when it is nil or blank.
func strOr(s *string, fallback string) string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return fallback
	}
	return strings.TrimSpace(*s)
}

// fleetListRow is the row shape shared by the follow-up and shutdown list tools.
type fleetListRow struct {
	ElevatorID     int     `json:"elevator_id"`
	Location       *string `json:"location"`
	Status         *string `json:"status"`
	LatestDate     *string `json:"latest_inspection_date"`
	Outcome        *string `json:"outcome"`
	InspectionType *string `json:"inspection_type"`
}

// writeFleetListRows appends one bullet line per elevator to b.
func writeFleetListRows(b *strings.Builder, rows []fleetListRow) {
	if len(rows) == 0 {
		return
	}
	b.WriteString("\n")
	for _, r := range rows {
		fmt.Fprintf(b, "- Elevator %d — %s — %s (last inspection %s)\n",
			r.ElevatorID,
			strOr(r.Location, "unknown location"),
			strOr(r.Outcome, "unknown outcome"),
			strOr(r.LatestDate, "no date"),
		)
	}
}

// ── Per-tool formatters ─────────────────────────────────────────────────────

// formatRiskBlock formats a get_elevator_risk payload. The three branches mirror
// the "Risk data rules" in the data agent prompt: not found, found but no
// prediction, and a full prediction (explanation shown only when present).
func formatRiskBlock(jsonText string) (string, bool) {
	var p struct {
		ElevatorFound   bool     `json:"elevator_found"`
		PredictionFound bool     `json:"prediction_found"`
		ElevatorID      int      `json:"elevator_id"`
		RiskScore       *float64 `json:"risk_score"`
		RiskLevel       string   `json:"risk_level"`
		RiskExplanation *string  `json:"risk_explanation"`
		ModelVersion    string   `json:"model_version"`
		PredictionDate  string   `json:"prediction_date"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}

	if !p.ElevatorFound {
		return notFoundBlock(p.ElevatorID), true
	}
	if !p.PredictionFound {
		return fmt.Sprintf("Source: live fleet database\n\nElevator %d has no risk prediction. The model scores only the highest-risk devices in the fleet.", p.ElevatorID), true
	}

	srcLine := sourceLine("predictions")
	var meta []string
	if p.ModelVersion != "" {
		meta = append(meta, "model "+p.ModelVersion)
	}
	if p.PredictionDate != "" {
		meta = append(meta, p.PredictionDate)
	}
	if len(meta) > 0 {
		srcLine += " (" + strings.Join(meta, ", ") + ")"
	}

	var b strings.Builder
	b.WriteString(srcLine)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Elevator: %d\n", p.ElevatorID)
	if p.RiskLevel != "" {
		fmt.Fprintf(&b, "Risk level: %s\n", p.RiskLevel)
	}
	if p.RiskScore != nil {
		b.WriteString("Score: " + strconv.FormatFloat(*p.RiskScore, 'f', 2, 64) + "\n")
	}
	if p.RiskExplanation != nil && strings.TrimSpace(*p.RiskExplanation) != "" {
		fmt.Fprintf(&b, "Explanation: %s\n", strings.TrimSpace(*p.RiskExplanation))
	}

	return strings.TrimRight(b.String(), "\n"), true
}

// formatFleetStats formats a get_fleet_stats payload.
func formatFleetStats(jsonText string) (string, bool) {
	var p struct {
		TotalElevators   int `json:"total_elevators"`
		RiskDistribution struct {
			Low     int `json:"low"`
			Medium  int `json:"medium"`
			High    int `json:"high"`
			Unknown int `json:"unknown"`
		} `json:"risk_distribution"`
		PassRate float64        `json:"inspection_pass_rate_pct"`
		Types    map[string]int `json:"equipment_type_distribution"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}

	var b strings.Builder
	b.WriteString(sourceLine("fleet-wide aggregate"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Total elevators: %d\n", p.TotalElevators)
	fmt.Fprintf(&b, "Risk — low: %d, medium: %d, high: %d, unknown: %d\n",
		p.RiskDistribution.Low, p.RiskDistribution.Medium, p.RiskDistribution.High, p.RiskDistribution.Unknown)
	fmt.Fprintf(&b, "Inspection pass rate: %s%%\n", strconv.FormatFloat(p.PassRate, 'f', -1, 64))

	if len(p.Types) > 0 {
		// Map order is random in Go — sort by count (desc) then name for a
		// stable, deterministic block.
		type kv struct {
			name string
			cnt  int
		}
		items := make([]kv, 0, len(p.Types))
		for k, v := range p.Types {
			items = append(items, kv{k, v})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].cnt != items[j].cnt {
				return items[i].cnt > items[j].cnt
			}
			return items[i].name < items[j].name
		})
		parts := make([]string, 0, len(items))
		for _, it := range items {
			parts = append(parts, fmt.Sprintf("%s: %d", it.name, it.cnt))
		}
		b.WriteString("Equipment types: " + strings.Join(parts, ", ") + "\n")
	}

	return strings.TrimRight(b.String(), "\n"), true
}

// formatInspectionHistory formats a get_inspection_history payload.
func formatInspectionHistory(jsonText string) (string, bool) {
	var p struct {
		Found       bool `json:"found"`
		ElevatorID  int  `json:"elevator_id"`
		Total       int  `json:"total_returned"`
		Inspections []struct {
			InspectionType string  `json:"inspection_type"`
			LatestDate     *string `json:"latest_inspection_date"`
			Outcome        *string `json:"outcome"`
		} `json:"inspections"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}
	if !p.Found {
		return notFoundBlock(p.ElevatorID), true
	}

	var b strings.Builder
	b.WriteString(sourceLine("inspections"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Elevator: %d\n", p.ElevatorID)
	fmt.Fprintf(&b, "Inspections found: %d\n", p.Total)
	if len(p.Inspections) > 0 {
		b.WriteString("\n")
		for _, ins := range p.Inspections {
			typ := ins.InspectionType
			if strings.TrimSpace(typ) == "" {
				typ = "Unknown type"
			}
			fmt.Fprintf(&b, "- %s — %s — %s\n",
				strOr(ins.LatestDate, "no date"), typ, strOr(ins.Outcome, "no outcome"))
		}
	}
	return strings.TrimRight(b.String(), "\n"), true
}

// formatElevatorIncidents formats a get_elevator_incidents payload.
func formatElevatorIncidents(jsonText string) (string, bool) {
	var p struct {
		Found      bool `json:"found"`
		ElevatorID int  `json:"elevator_id"`
		Total      int  `json:"total_returned"`
		Incidents  []struct {
			IncidentID     int     `json:"incident_id"`
			DateOccurrence *string `json:"date_of_occurrence"`
			Category       *string `json:"category"`
			Summary        *string `json:"incident_summary"`
			InjurySeverity *string `json:"injury_severity"`
			FatalInjury    bool    `json:"fatal_injury"`
		} `json:"incidents"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}
	if !p.Found {
		return notFoundBlock(p.ElevatorID), true
	}

	var b strings.Builder
	b.WriteString(sourceLine("incidents"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Elevator: %d\n", p.ElevatorID)
	fmt.Fprintf(&b, "Incidents found: %d\n", p.Total)
	if len(p.Incidents) > 0 {
		b.WriteString("\n")
		for _, inc := range p.Incidents {
			fmt.Fprintf(&b, "- Incident #%d (%s) — category: %s, injury: %s",
				inc.IncidentID,
				strOr(inc.DateOccurrence, "no date"),
				strOr(inc.Category, "uncategorized"),
				strOr(inc.InjurySeverity, "none"))
			if inc.FatalInjury {
				b.WriteString(", fatal")
			}
			b.WriteString("\n")
			if inc.Summary != nil && strings.TrimSpace(*inc.Summary) != "" {
				fmt.Fprintf(&b, "  Summary: %s\n", strings.TrimSpace(*inc.Summary))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n"), true
}

// formatFollowup formats a get_elevators_needing_followup payload.
func formatFollowup(jsonText string) (string, bool) {
	var p struct {
		Count     int            `json:"count"`
		Elevators []fleetListRow `json:"elevators"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}

	var b strings.Builder
	b.WriteString(sourceLine("most recent inspection per elevator"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Elevators needing follow-up: %d\n", p.Count)
	writeFleetListRows(&b, p.Elevators)
	return strings.TrimRight(b.String(), "\n"), true
}

// formatShutdown formats a get_tssa_shutdown_elevators payload. It keeps the
// tool's "note" because it explains how shutdown status is derived.
//
// The header splits the count by outcome type so the model's intro sentence
// cannot conflate "Follow up" entries (compliance issue, not a shutdown order)
// with genuine "Vol Shut Down" / "Shutdown" entries. All rows are still listed
// below so operators can see the full set.
func formatShutdown(jsonText string) (string, bool) {
	var p struct {
		Count     int            `json:"count"`
		Note      string         `json:"note"`
		Elevators []fleetListRow `json:"elevators"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}

	var shutdownCount, followupCount, otherCount int
	for _, r := range p.Elevators {
		outcome := strings.ToLower(strOr(r.Outcome, ""))
		switch {
		case strings.Contains(outcome, "shut down") || outcome == "shutdown":
			shutdownCount++
		case strings.Contains(outcome, "follow up") || outcome == "follow-up":
			followupCount++
		default:
			otherCount++
		}
	}

	var b strings.Builder
	b.WriteString(sourceLine("most recent inspection per elevator"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Elevators with non-passing outcomes: %d\n", p.Count)
	fmt.Fprintf(&b, "  Voluntarily shut down: %d\n", shutdownCount)
	fmt.Fprintf(&b, "  Requiring follow-up: %d\n", followupCount)
	if otherCount > 0 {
		fmt.Fprintf(&b, "  Other non-passing: %d\n", otherCount)
	}
	if strings.TrimSpace(p.Note) != "" {
		fmt.Fprintf(&b, "Note: %s\n", strings.TrimSpace(p.Note))
	}
	writeFleetListRows(&b, p.Elevators)
	return strings.TrimRight(b.String(), "\n"), true
}

// formatIncidentCount formats a get_incident_count_last_year payload.
func formatIncidentCount(jsonText string) (string, bool) {
	var p struct {
		Total       int `json:"total_incidents"`
		Fatal       int `json:"fatal_incidents"`
		Injury      int `json:"injury_incidents"`
		YearQueried int `json:"year_queried"`
	}
	if err := json.Unmarshal([]byte(jsonText), &p); err != nil {
		return "", false
	}

	var b strings.Builder
	b.WriteString(sourceLine("incidents (aggregate)"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Year: %d\n", p.YearQueried)
	fmt.Fprintf(&b, "Total incidents: %d\n", p.Total)
	fmt.Fprintf(&b, "With injury: %d\n", p.Injury)
	fmt.Fprintf(&b, "Fatal: %d\n", p.Fatal)
	return strings.TrimRight(b.String(), "\n"), true
}
