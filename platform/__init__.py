"""
Makes platform/ a Python package so `py -m platform.mcp.server` resolves
the local package instead of the stdlib platform module.

Forwards all attribute lookups to the real stdlib platform module so that
third-party packages (attrs, jsonschema, etc.) that call
platform.python_implementation() or platform.system() keep working.
"""
import os as _os
import sys as _sys
import importlib.util as _util

_stdlib = None


def _load_stdlib():
    here = _os.path.dirname(_os.path.abspath(__file__))
    for p in _sys.path:
        if not p:
            continue
        if _os.path.abspath(p) == here:
            continue
        candidate = _os.path.join(p, "platform.py")
        if _os.path.isfile(candidate):
            spec = _util.spec_from_file_location("_stdlib_platform", candidate)
            if spec and spec.loader:
                mod = _util.module_from_spec(spec)
                spec.loader.exec_module(mod)
                return mod
    return None


def __getattr__(name: str):
    global _stdlib
    if _stdlib is None:
        _stdlib = _load_stdlib()
    if _stdlib is not None and hasattr(_stdlib, name):
        return getattr(_stdlib, name)
    raise AttributeError(f"module 'platform' has no attribute {name!r}")
