# Export Evergage recipe list and details.
# Set RECIPE_COOKIES (and optionally RECIPE_BASE_URL) before running.

PYTHON ?= python3
VENV   ?= .venv
BIN    := $(VENV)/bin
PIP    := $(BIN)/pip
# Use venv python if it exists, otherwise system python
PY     := $(if $(wildcard $(BIN)/python),$(BIN)/python,$(PYTHON))

.PHONY: install run list-only help

help:
	@echo "Export recipes from Evergage API"
	@echo ""
	@echo "Usage:"
	@echo "  make install     Install dependencies (optional, use a venv)"
	@echo "  make run         Fetch list + all details, write to recipes.json (requires RECIPE_COOKIES)"
	@echo "  make list-only   Fetch only the recipe list (requires RECIPE_COOKIES)"
	@echo ""
	@echo "Environment:"
	@echo "  RECIPE_COOKIES   Cookie string from browser (JSESSIONID=...; AWSALBTGCORS=...). Required."
	@echo "  RECIPE_BASE_URL  API base URL (default: https://yourinstance.evergage.com)"
	@echo ""
	@echo "Example:"
	@echo "  export RECIPE_COOKIES='JSESSIONID=xxx; AWSALBTGCORS=yyy'"
	@echo "  make run"

install:
	$(PYTHON) -m venv $(VENV)
	$(PIP) install -r requirements.txt

run:
	@if [ -z "$$RECIPE_COOKIES" ]; then echo "Error: set RECIPE_COOKIES (see make help)"; exit 1; fi
	$(PY) main.py -o recipes.json --workers 12 --batch-size 25

list-only:
	@if [ -z "$$RECIPE_COOKIES" ]; then echo "Error: set RECIPE_COOKIES (see make help)"; exit 1; fi
	$(PY) main.py --list-only
