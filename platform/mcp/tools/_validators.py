"""
Shared input validation for all MCP tools.

AND-107 security requirement: treat every tool input as untrusted user input,
the same way a web API would. All tools import from here — never inline-validate.
"""

from datetime import date, datetime


def validate_elevator_id(value: int) -> int:
    if not isinstance(value, int) or isinstance(value, bool):
        raise ValueError("elevator_id must be an integer.")
    if value <= 0:
        raise ValueError("elevator_id must be a positive integer.")
    if value > 9_999_999:
        raise ValueError("elevator_id is out of the expected range (max 9,999,999).")
    return value


def validate_limit(value: int, max_val: int = 50) -> int:
    if not isinstance(value, int) or isinstance(value, bool):
        raise ValueError("limit must be an integer.")
    if value < 1:
        raise ValueError("limit must be at least 1.")
    if value > max_val:
        raise ValueError(f"limit must not exceed {max_val}.")
    return value


def validate_query_string(value: str, max_len: int = 500) -> str:
    if not isinstance(value, str):
        raise ValueError("query must be a string.")
    value = value.strip()
    if not value:
        raise ValueError("query must not be empty.")
    if len(value) > max_len:
        raise ValueError(f"query exceeds the {max_len}-character limit.")
    return value


def validate_inspection_date(value: str) -> date:
    if not isinstance(value, str):
        raise ValueError("inspection_date must be a string in YYYY-MM-DD format.")
    try:
        d = datetime.strptime(value.strip(), "%Y-%m-%d").date()
    except ValueError:
        raise ValueError("inspection_date must be in YYYY-MM-DD format (e.g. 2026-07-15).")
    if d < date.today():
        raise ValueError("inspection_date must not be in the past.")
    return d


def validate_reason(value: str) -> str:
    if not isinstance(value, str):
        raise ValueError("reason must be a string.")
    value = value.strip()
    if not value:
        raise ValueError("reason must not be empty.")
    if len(value) > 500:
        raise ValueError("reason exceeds the 500-character limit.")
    return value
