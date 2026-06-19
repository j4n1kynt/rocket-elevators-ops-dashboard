"""
Pydantic input models for all MCP tools that accept user-facing parameters.

Each model uses strict mode (no implicit type coercion) and explicit
@field_validator methods so that every error message is precise. Field()
carries only the default value and description — no gt/ge/le/min_length
constraints — keeping all validation logic in one visible place.

Three module-level helpers are shared across models:
  _check_elevator_id  — positive integer, bounded range
  _check_limit        — integer within [1, max_val]
  _strip_nonempty     — strips whitespace, rejects empty, enforces max length

Tools with no inputs (get_incident_count_last_year, get_fleet_stats) have no
corresponding model.
"""

from datetime import date, datetime

from pydantic import BaseModel, ConfigDict, Field, field_validator


# ── Shared validator helpers ───────────────────────────────────────────────────

def _check_elevator_id(v: int) -> int:
    if v <= 0:
        raise ValueError("elevator_id must be a positive integer.")
    if v > 9_999_999:
        raise ValueError("elevator_id must not exceed 9,999,999.")
    return v


def _check_limit(v: int, *, max_val: int) -> int:
    if v < 1:
        raise ValueError("limit must be at least 1.")
    if v > max_val:
        raise ValueError(f"limit must not exceed {max_val}.")
    return v


def _strip_nonempty(v: object, *, field: str, max_len: int) -> object:
    """Strip whitespace, reject blank string, enforce max length."""
    if not isinstance(v, str):
        return v  # non-str values fail Pydantic's strict type check
    stripped = v.strip()
    if not stripped:
        raise ValueError(f"{field} must not be empty.")
    if len(stripped) > max_len:
        raise ValueError(f"{field} must not exceed {max_len} characters.")
    return stripped


# ── Models ────────────────────────────────────────────────────────────────────

class GetElevatorIncidentsInput(BaseModel):
    model_config = ConfigDict(strict=True)

    elevator_id: int = Field(
        ...,
        description="Unique elevator ID (positive integer, max 9,999,999).",
    )
    limit: int = Field(
        20,
        description="Maximum number of incidents to return (1–100). Defaults to 20.",
    )

    @field_validator("elevator_id")
    @classmethod
    def validate_elevator_id(cls, v: int) -> int:
        return _check_elevator_id(v)

    @field_validator("limit")
    @classmethod
    def validate_limit(cls, v: int) -> int:
        return _check_limit(v, max_val=100)


class GetTssaShutdownElevatorsInput(BaseModel):
    model_config = ConfigDict(strict=True)

    limit: int = Field(
        50,
        description="Maximum number of elevators to return (1–200). Defaults to 50.",
    )

    @field_validator("limit")
    @classmethod
    def validate_limit(cls, v: int) -> int:
        return _check_limit(v, max_val=200)


class GetInspectionHistoryInput(BaseModel):
    model_config = ConfigDict(strict=True)

    elevator_id: int = Field(
        ...,
        description="Unique elevator ID (positive integer, max 9,999,999).",
    )
    limit: int = Field(
        20,
        description="Maximum number of inspections to return (1–100). Defaults to 20.",
    )

    @field_validator("elevator_id")
    @classmethod
    def validate_elevator_id(cls, v: int) -> int:
        return _check_elevator_id(v)

    @field_validator("limit")
    @classmethod
    def validate_limit(cls, v: int) -> int:
        return _check_limit(v, max_val=100)


class GetElevatorsNeedingFollowupInput(BaseModel):
    model_config = ConfigDict(strict=True)

    limit: int = Field(
        50,
        description="Maximum number of elevators to return (1–200). Defaults to 50.",
    )

    @field_validator("limit")
    @classmethod
    def validate_limit(cls, v: int) -> int:
        return _check_limit(v, max_val=200)


class GetElevatorRiskInput(BaseModel):
    model_config = ConfigDict(strict=True)

    elevator_id: int = Field(
        ...,
        description="Unique elevator ID (positive integer, max 9,999,999).",
    )

    @field_validator("elevator_id")
    @classmethod
    def validate_elevator_id(cls, v: int) -> int:
        return _check_elevator_id(v)


_ALLOWED_INSPECTION_TYPES = {"Periodic", "Followup", "Initial", "Incident", "Alteration"}


class ScheduleInspectionInput(BaseModel):
    model_config = ConfigDict(strict=True)

    elevator_id: int = Field(
        ...,
        description="Unique elevator ID (positive integer, max 9,999,999).",
    )
    inspection_date: date = Field(
        ...,
        description="Target inspection date in YYYY-MM-DD format. Must not be in the past.",
    )
    reason: str = Field(
        ...,
        description="Brief reason for scheduling the inspection (max 500 characters).",
    )
    inspection_type: str = Field(
        "",
        description=(
            "Type of inspection. Must be one of: Periodic, Followup, Initial, Incident, Alteration. "
            "Leave empty to default to Periodic."
        ),
    )
    confirmed: bool = Field(
        False,
        description=(
            "Must be True to commit the write. "
            "Pass False (default) to receive a confirmation preview without writing."
        ),
    )

    @field_validator("elevator_id")
    @classmethod
    def validate_elevator_id(cls, v: int) -> int:
        return _check_elevator_id(v)

    @field_validator("inspection_date", mode="before")
    @classmethod
    def validate_inspection_date(cls, v: object) -> date:
        if isinstance(v, date):
            d = v
        elif isinstance(v, str):
            try:
                d = datetime.strptime(v.strip(), "%Y-%m-%d").date()
            except ValueError:
                raise ValueError(
                    "inspection_date must be in YYYY-MM-DD format (e.g. 2026-07-15)."
                )
        else:
            raise ValueError("inspection_date must be a string in YYYY-MM-DD format.")
        if d < date.today():
            raise ValueError("inspection_date must not be in the past.")
        return d

    @field_validator("inspection_type", mode="before")
    @classmethod
    def validate_inspection_type(cls, v: object) -> str:
        if not isinstance(v, str):
            raise ValueError("inspection_type must be a string.")
        if v == "":
            return v
        if v not in _ALLOWED_INSPECTION_TYPES:
            allowed = ", ".join(sorted(_ALLOWED_INSPECTION_TYPES))
            raise ValueError(
                f"inspection_type must be one of: {allowed}. Got: {v!r}."
            )
        return v

    @field_validator("reason", mode="before")
    @classmethod
    def validate_reason(cls, v: object) -> object:
        return _strip_nonempty(v, field="reason", max_len=500)


class SearchMaintenanceDocsInput(BaseModel):
    model_config = ConfigDict(strict=True)

    query: str = Field(
        ...,
        description="Natural-language search query for maintenance documents (max 500 characters).",
    )
    n_results: int = Field(
        5,
        description="Number of results to return (1–20). Defaults to 5.",
    )

    @field_validator("query", mode="before")
    @classmethod
    def validate_query(cls, v: object) -> object:
        return _strip_nonempty(v, field="query", max_len=500)

    @field_validator("n_results")
    @classmethod
    def validate_n_results(cls, v: int) -> int:
        return _check_limit(v, max_val=20)


class SearchIncidentNarrativesInput(BaseModel):
    model_config = ConfigDict(strict=True)

    query: str = Field(
        ...,
        description="Full-text search query for incident narrative text (max 200 characters).",
    )
    limit: int = Field(
        5,
        description="Maximum number of incidents to return (1–20). Defaults to 5.",
    )

    @field_validator("query", mode="before")
    @classmethod
    def validate_query(cls, v: object) -> object:
        return _strip_nonempty(v, field="query", max_len=200)

    @field_validator("limit")
    @classmethod
    def validate_limit(cls, v: int) -> int:
        return _check_limit(v, max_val=20)
