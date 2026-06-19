"""
Startup healthcheck for the MCP server's PostgreSQL connection.

Calls init_pool() (which runs SELECT 1 + sequence DDL), then closes cleanly.
Exits 0 on success, 1 on failure.

Usage:
  py -3 -m platform.mcp.healthcheck
  python platform/mcp/healthcheck.py
"""

import asyncio
import sys

from platform.mcp.db import close_pool, init_pool


async def _check() -> None:
    await init_pool()
    await close_pool()


if __name__ == "__main__":
    try:
        asyncio.run(_check())
        print("OK: PostgreSQL connection healthy.")
        sys.exit(0)
    except Exception as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        sys.exit(1)
