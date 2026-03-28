# Contributing

## Local Setup

Use a virtual environment and install development dependencies:

```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt -r requirements-integration.txt
pip install black isort flake8 mypy
```

## Pre-Commit Hook

This repository ships a version-controlled Git hook at `.githooks/pre-commit`.

Enable it once per clone:

```bash
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit
```

On each commit, the hook runs these checks on staged Python files:

- `isort`
- `black`
- `flake8`
- `mypy`

If formatters update files, the hook re-stages those Python files automatically.

## Manual Checks

Run these before opening or updating a PR:

```bash
mypy tests
pytest -q tests/test_app.py
```