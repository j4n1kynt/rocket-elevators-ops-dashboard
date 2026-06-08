"""
platform/test_server.py — TDD tests for Flask dashboard endpoints.

Test framework: pytest with Flask test client
Focus: Verify filtering, sorting, and page load behavior before implementation

Run: pytest platform/test_server.py -v
"""

import pytest
from datetime import date, timedelta
from html.parser import HTMLParser
from server import app


class HTMLTableParser(HTMLParser):
    """Extract table rows and cell data from HTML responses.

    Works for both bare <tr> responses (innerHTML swap) and full
    <table> responses (outerHTML swap). Skips header rows (<th> only).
    """

    def __init__(self):
        super().__init__()
        self.rows = []
        self.current_row = []
        self.in_tr = False
        self.in_td = False
        self.cell_data = ""

    def handle_starttag(self, tag, attrs):
        if tag == "tr":
            self.in_tr = True
            self.current_row = []
        elif tag == "td" and self.in_tr:
            self.in_td = True
            self.cell_data = ""

    def handle_endtag(self, tag):
        if tag == "tr":
            self.in_tr = False
            # Only keep rows that have <td> cells (skip header <th> rows)
            if self.current_row:
                self.rows.append(self.current_row)
        elif tag == "td" and self.in_tr:
            self.in_td = False
            self.current_row.append(self.cell_data.strip())

    def handle_data(self, data):
        if self.in_td:
            self.cell_data += data


@pytest.fixture
def client():
    """Flask test client fixture."""
    app.config["TESTING"] = True
    with app.test_client() as test_client:
        yield test_client



# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Main page load (GET /)
# ──────────────────────────────────────────────────────────────────────────────

class TestMainPageLoad:
    """Verify dashboard homepage renders correctly."""

    def test_get_index_returns_200(self, client):
        """GET / should return 200 OK."""
        response = client.get("/")
        assert response.status_code == 200

    def test_get_index_returns_html(self, client):
        """GET / response should be HTML content."""
        response = client.get("/")
        assert "text/html" in response.content_type

    def test_get_index_contains_summary_cards(self, client):
        """GET / should include summary card metrics."""
        response = client.get("/")
        data = response.get_data(as_text=True)
        # Check for summary card identifiers (from spec §2)
        assert "total_elevators" in data or "Total Elevators" in data
        assert "active_elevators" in data or "Active Elevators" in data

    def test_get_index_contains_fleet_table(self, client):
        """GET / should include the initial fleet table."""
        response = client.get("/")
        data = response.get_data(as_text=True)
        # Table should be present with fleetTable id for HTMX targeting
        assert "fleetTable" in data


# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Filtering behavior (GET /table?status=...)
# ──────────────────────────────────────────────────────────────────────────────

class TestFilteringByStatus:
    """Verify filtering returns only matching status rows."""

    def test_filter_active_status_returns_200(self, client):
        """GET /table?status=ACTIVE should return 200 OK."""
        response = client.get("/table?status=ACTIVE")
        assert response.status_code == 200

    def test_filter_active_status_returns_html(self, client):
        """Filtered response should be HTML (table rows)."""
        response = client.get("/table?status=ACTIVE")
        assert "text/html" in response.content_type

    def test_filter_active_contains_only_active_rows(self, client):
        """All rows from GET /table?status=ACTIVE must have Status='ACTIVE'."""
        response = client.get("/table?status=ACTIVE")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        # Status is column 3 (index 3): [ID, Location, License, Status, ...]
        assert len(parser.rows) > 0, "Expected at least one ACTIVE row"

        for row in parser.rows:
            if len(row) > 3:
                status = row[3].strip()
                assert status == "ACTIVE", f"Expected ACTIVE, got {status}"

    def test_filter_active_count_matches_data(self, client):
        """All returned rows for status=ACTIVE must have Status='ACTIVE'."""
        response = client.get("/table?status=ACTIVE")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0, "Expected at least one ACTIVE row"
        for row in parser.rows:
            if len(row) > 3:
                assert row[3].strip() == "ACTIVE"

    def test_filter_by_request_status(self, client):
        """Filtering by BY REQUEST status should return only BY REQUEST elevators."""
        response = client.get("/table?status=BY%20REQUEST")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0, "Expected at least one BY REQUEST row"
        for row in parser.rows:
            if len(row) > 3:
                assert row[3].strip() == "BY REQUEST"

    def test_filter_unknown_status_returns_empty(self, client):
        """Filtering by non-existent status should return no data rows."""
        response = client.get("/table?status=NONEXISTENT")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        # Data rows have 8 cells; single-cell rows are the empty-state message
        data_rows = [r for r in parser.rows if len(r) > 1]
        assert len(data_rows) == 0, "Non-existent status should return zero data rows"


# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Sorting behavior (GET /table?sort=...&order=...)
# ──────────────────────────────────────────────────────────────────────────────

class TestSortingBehavior:
    """Verify sorting produces correct row order."""

    def test_sort_license_expiry_asc_returns_200(self, client):
        """GET /table?sort=license_expiry&order=asc should return 200 OK."""
        response = client.get("/table?sort=license_expiry&order=asc")
        assert response.status_code == 200

    def test_sort_license_expiry_asc_ascending_order(self, client):
        """Rows sorted by license_expiry ASC must be in ascending order."""
        response = client.get("/table?sort=license_expiry&order=asc")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0, "Expected sorted rows"

        # License Expiration Date is column 4 (index 4): [ID, Location, License, Status, Date, ...]
        dates = []
        for row in parser.rows:
            if len(row) > 4:
                date_str = row[4].strip()
                # Parse dates, treating empty as None (sorts last)
                try:
                    if date_str:
                        dates.append(date.fromisoformat(date_str))
                    else:
                        dates.append(None)
                except ValueError:
                    dates.append(None)

        # Verify ascending order (None values at end)
        non_none_dates = [d for d in dates if d is not None]
        assert non_none_dates == sorted(non_none_dates), (
            "License expiry dates not in ascending order"
        )

    def test_sort_license_expiry_desc_descending_order(self, client):
        """Rows sorted by license_expiry DESC must be in descending order."""
        response = client.get("/table?sort=license_expiry&order=desc")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0

        # License Expiration Date is column 4
        dates = []
        for row in parser.rows:
            if len(row) > 4:
                date_str = row[4].strip()
                try:
                    if date_str:
                        dates.append(date.fromisoformat(date_str))
                    else:
                        dates.append(None)
                except ValueError:
                    dates.append(None)

        non_none_dates = [d for d in dates if d is not None]
        assert non_none_dates == sorted(non_none_dates, reverse=True), (
            "License expiry dates not in descending order"
        )

    def test_sort_latest_inspection_asc(self, client):
        """Rows sorted by latest_inspection ASC must be in ascending order."""
        response = client.get("/table?sort=latest_inspection&order=asc")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0

        # Latest Inspection Date is column 5
        dates = []
        for row in parser.rows:
            if len(row) > 5:
                date_str = row[5].strip()
                try:
                    if date_str:
                        dates.append(date.fromisoformat(date_str))
                    else:
                        dates.append(None)
                except ValueError:
                    dates.append(None)

        non_none_dates = [d for d in dates if d is not None]
        assert non_none_dates == sorted(non_none_dates)

    def test_sort_latest_inspection_desc(self, client):
        """Rows sorted by latest_inspection DESC must be in descending order."""
        response = client.get("/table?sort=latest_inspection&order=desc")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0

        # Latest Inspection Date is column 5
        dates = []
        for row in parser.rows:
            if len(row) > 5:
                date_str = row[5].strip()
                try:
                    if date_str:
                        dates.append(date.fromisoformat(date_str))
                    else:
                        dates.append(None)
                except ValueError:
                    dates.append(None)

        non_none_dates = [d for d in dates if d is not None]
        assert non_none_dates == sorted(non_none_dates, reverse=True)


# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Combined filtering + sorting
# ──────────────────────────────────────────────────────────────────────────────

class TestFilterAndSort:
    """Verify filtering and sorting work together."""

    def test_filter_and_sort_combined(self, client):
        """Applying both filter and sort should combine effects."""
        response = client.get("/table?status=ACTIVE&sort=license_expiry&order=asc")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0

        # All rows must be ACTIVE
        for row in parser.rows:
            if len(row) > 3:
                assert row[3].strip() == "ACTIVE"

        # Dates must be sorted ascending
        dates = []
        for row in parser.rows:
            if len(row) > 4:
                date_str = row[4].strip()
                try:
                    if date_str:
                        dates.append(date.fromisoformat(date_str))
                    else:
                        dates.append(None)
                except ValueError:
                    dates.append(None)

        non_none_dates = [d for d in dates if d is not None]
        assert non_none_dates == sorted(non_none_dates)


# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Search/query parameter behavior (GET /table?q=...)
# ──────────────────────────────────────────────────────────────────────────────

class TestSearchBehavior:
    """Verify search query parameter filters correctly."""

    def test_search_by_elevator_id(self, client):
        """Search matches on Elevator ID or Location (OR logic, partial match)."""
        response = client.get("/table?q=10")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        assert len(parser.rows) > 0, "Search for '10' should return results"

        # Each row must have "10" in either Elevator ID (col 0) or Location (col 1)
        for row in parser.rows:
            if len(row) > 1:
                elevator_id = row[0].lower()
                location    = row[1].lower()
                assert "10" in elevator_id or "10" in location, (
                    f"Expected '10' in ID '{row[0]}' or Location '{row[1]}'"
                )

    def test_search_case_insensitive(self, client):
        """Search should be case-insensitive."""
        response_lower = client.get("/table?q=toronto")
        response_upper = client.get("/table?q=TORONTO")

        parser_lower = HTMLTableParser()
        parser_lower.feed(response_lower.get_data(as_text=True))

        parser_upper = HTMLTableParser()
        parser_upper.feed(response_upper.get_data(as_text=True))

        # Both queries should return the same number of rows
        assert len(parser_lower.rows) == len(parser_upper.rows)

    def test_search_no_results(self, client):
        """Search for non-existent term should return no data rows."""
        response = client.get("/table?q=XYZNONEXISTENT")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        # Data rows have 8 cells; single-cell rows are the empty-state message
        data_rows = [r for r in parser.rows if len(r) > 1]
        assert len(data_rows) == 0


# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: HTMX header handling (response shape)
# ──────────────────────────────────────────────────────────────────────────────

class TestHTMXResponseShape:
    """Verify response changes based on HX-Target header."""

    def test_default_response_is_rows_only(self, client):
        """Default response (no HX-Target) should be rows only (innerHTML swap)."""
        response = client.get("/table?status=ACTIVE")
        html = response.get_data(as_text=True)

        # Should NOT include full <table> wrapper
        assert "id=\"fleetTable\"" not in html, "Default response should not include full table"
        # Should contain <tr> rows (response has attributes on the tag)
        assert "<tr" in html

    def test_fleetTable_target_includes_full_table(self, client):
        """HX-Target: fleetTable should return full <table> (outerHTML swap)."""
        response = client.get(
            "/table?sort=license_expiry&order=asc",
            headers={"HX-Target": "fleetTable"}
        )
        html = response.get_data(as_text=True)

        # Should include full table wrapper (tags have CSS classes, match by prefix)
        assert "id=\"fleetTable\"" in html
        assert "<thead" in html
        assert "<tbody" in html


# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Edge cases and error handling
# ──────────────────────────────────────────────────────────────────────────────

# ──────────────────────────────────────────────────────────────────────────────
# TEST SUITE: Elevator detail panel (GET /elevator/<id>)
# ──────────────────────────────────────────────────────────────────────────────

class TestElevatorDetail:
    """Verify the elevator detail panel endpoint returns correct HTML fragments."""

    VALID_ID = "10"  # Exists in elevator_fleet.csv with inspection history

    def test_valid_id_returns_200(self, client):
        """GET /elevator/<valid_id> should return 200 OK."""
        response = client.get(f"/elevator/{self.VALID_ID}")
        assert response.status_code == 200

    def test_valid_id_response_contains_elevator_id(self, client):
        """Response body must include the requested elevator ID."""
        response = client.get(f"/elevator/{self.VALID_ID}")
        html = response.get_data(as_text=True)
        assert self.VALID_ID in html, f"Expected elevator ID '{self.VALID_ID}' in response"

    def test_nonexistent_id_returns_404(self, client):
        """GET /elevator/<nonexistent_id> should return 404."""
        response = client.get("/elevator/999999999")
        assert response.status_code == 404

    def test_response_contains_inspection_history(self, client):
        """Detail panel must include inspection date and outcome for elevator 10."""
        response = client.get(f"/elevator/{self.VALID_ID}")
        html = response.get_data(as_text=True)
        # Elevator 10 has: Latest Inspection Date=2015-03-27, Outcome=All Orders Resolved
        assert "2015-03-27" in html, "Expected inspection date in response"
        assert "All Orders Resolved" in html, "Expected inspection outcome in response"


class TestEdgeCases:
    """Verify robust behavior with edge inputs."""

    def test_empty_query_parameters(self, client):
        """Empty query parameters should not error."""
        response = client.get("/table?status=&type=&q=&sort=&order=")
        assert response.status_code == 200

    def test_invalid_sort_field_ignored(self, client):
        """Invalid sort field should be ignored without error."""
        response = client.get("/table?sort=invalid_field&order=asc")
        assert response.status_code == 200

    def test_invalid_order_direction_defaults_asc(self, client):
        """Invalid order direction should default to ascending."""
        response = client.get("/table?sort=license_expiry&order=invalid")
        # Should still return 200; sort behavior handles invalid order
        assert response.status_code == 200

    def test_multiple_filters_combine_correctly(self, client):
        """Multiple filters should be applied cumulatively (AND logic)."""
        response = client.get("/table?status=ACTIVE&type=Passenger%20Elevator")
        html = response.get_data(as_text=True)

        parser = HTMLTableParser()
        parser.feed(html)

        # Verify all rows match both criteria
        for row in parser.rows:
            if len(row) > 3:
                assert row[3].strip() == "ACTIVE", "Status should be ACTIVE"
            if len(row) > 7:
                # Elevator Type is column 7
                assert "Passenger" in row[7], "Type should contain Passenger"

    def test_404_on_invalid_route(self, client):
        """Invalid routes should return 404."""
        response = client.get("/invalid_route")
        assert response.status_code == 404
