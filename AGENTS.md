# AGENTS.md

This repository is a Python 3 network test server (TCP/UDP/HTTP/MQTT).
Use this guide for build, lint, tests, and local conventions.

## Quick Facts
- Language: Python 3.11+
- No external runtime dependencies (stdlib only)
- CI: GitHub Actions runs `pytest tests/` (see `.github/workflows/python.yml`)

## Run
- Run help: `python yourtestsrv.py --help`
- Run all servers: `python yourtestsrv.py serve-all --config config.json`
- Run all TLS servers: `python yourtestsrv.py serve-all-tls --config config.json`

## Tests
- Install test runner: `pip install pytest`
- Run all tests: `python -m pytest tests/ -v`
- Run a single test file: `python -m pytest tests/test_tcp.py -v`
- Run a single test by name: `python -m pytest tests/test_tcp.py::TestTCPEcho::test_echo -v`
- Repeat a test (flakiness check): `python -m pytest tests/test_udp.py::TestUDPEcho::test_echo -v --count=10`

## Lint / Format
- Format: `python -m py_compile yourtestsrv.py yourtestsrv/*.py tests/*.py`
- Style guide: PEP 8

## Project Layout
- `yourtestsrv.py`: CLI entry point and server startup.
- `yourtestsrv/config.py`: config types + JSON parsing (supports Go-style duration strings).
- `yourtestsrv/tcp_server.py`, `udp_server.py`, `http_server.py`, `mqtt_server.py`: protocol servers.
- `tests/`: pytest test suite.
- `config.json`: default config example used by CLI.

## Code Style Guidelines

### Imports
- Use standard PEP 8 import grouping: stdlib, blank line, local.
- Avoid unnecessary aliases.

### Formatting
- Follow PEP 8; prefer 4-space indentation.
- Use f-strings for log messages.

### Naming
- `snake_case` for all names; `PascalCase` for classes.
- Avoid abbreviations unless standard (e.g., `ctx`, `cfg`, `conn`).

### Concurrency / Cancellation
- Use `threading.Event` (stop_event) for graceful shutdown.
- Each connection is handled in a daemon thread.
- Server accept loops check `stop_event.is_set()` with a 1-second socket timeout.

### Networking Behavior
- Default listeners bind to `0.0.0.0` and use configured ports.
- TLS listeners require `cert.pem` and `key.pem`; tests generate temp certs via `cryptography`.
- All TLS server contexts enforce a minimum of TLS 1.2.

### Testing Conventions
- Tests live in `tests/test_<protocol>.py`.
- Use ephemeral ports (bind to port 0, read assigned port) to avoid conflicts.
- For network readiness, poll with a short deadline (`wait_tcp` helper).
- TLS tests are skipped if the `cryptography` package is not available.

### Logging
- Use `logging.getLogger(__name__)` in each module.
- Keep messages short and actionable.

### JSON / Config
- Config is JSON with snake_case keys; see `yourtestsrv/config.py`.
- Duration values accept Go-style strings (`"200ms"`, `"5s"`, `"1m30s"`).
- Do not add external deps for config parsing.

## Cursor / Copilot Rules
- No `.cursor/rules/`, `.cursorrules`, or `.github/copilot-instructions.md`
  were found in this repository at the time of writing.

## When Adding New Features
- Keep new protocol modules inside `yourtestsrv/<protocol>_server.py`.
- Add tests in `tests/test_<protocol>.py` covering basic behavior plus one edge case.
- Update README usage examples if CLI flags change.

## Common Commands Summary
- Run: `python yourtestsrv.py serve-all --config config.json`
- Test all: `python -m pytest tests/ -v`
- Test single: `python -m pytest tests/test_tcp.py::TestTCPEcho::test_echo -v`
