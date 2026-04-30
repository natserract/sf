# sfmc-retention

A fast, concurrent CLI tool that sets **7-day data retention** on Salesforce Marketing Cloud (SFMC) Data Extensions used in Journeys, with isolated step commands and PostgreSQL persistence.

## What it does

1. Fetches all Journeys (paginated, all versions)
2. Resolves each Journey's trigger → Event Definition → Data Extension
3. Fetches DE details and exports a step snapshot
4. Updates each DE's retention to **7 days** (row-based, concurrent)
5. Re-fetches updated DEs
6. Persists all step outputs into PostgreSQL tables keyed by `run_id` + `business_unit_id`

---

## Installation

```bash
# Clone
git clone <repo>
cd sfmc-retention

# Build (requires Go 1.21+)
make build

# Or cross-compile for all platforms
make build-all
```

---

## Authentication — where to find each credential

All credentials come from browser DevTools on the SFMC UI (Network tab):

| Flag | Env var | Where to find it |
|---|---|---|
| `--jb-host` | `SFMC_JB_HOST` | Hostname of JB requests, e.g. `jbinteractions.s12.marketingcloudapps.com` |
| `--jb-cookie` | `SFMC_JB_COOKIE` | Cookie header from any `/fuelapi/interaction/` request |
| `--jb-csrf` | `SFMC_JB_CSRF` | `X-CSRF-Token` header from any JB request |
| `--mc-host` | `SFMC_MC_HOST` | Hostname of MC requests, e.g. `mc.s12.marketingcloudapps.com` |
| `--mc-cookie` | `SFMC_MC_COOKIE` | Cookie header from the PATCH `/contactsmeta/fuelapi/internal/v1/customobjects/` request |
| `--mc-csrf` | `SFMC_MC_CSRF` | `X-CSRF-Token` header from the same PATCH request |
| `--bearer` | `SFMC_BEARER` | `authorization: Bearer XXXXXXX` from the PATCH request |

> **Tip:** Export all as environment variables to avoid passing them on every run:
> ```bash
> export SFMC_JB_HOST="jbinteractions.s12.marketingcloudapps.com"
> export SFMC_JB_COOKIE="ff6288631bbb74b54ce9223b62465d85=s%3A..."
> export SFMC_JB_CSRF="0BMloAMA-k27yJ6wq4YUjTqShycaAhMqueUA"
> export SFMC_MC_HOST="mc.s12.marketingcloudapps.com"
> export SFMC_MC_COOKIE="567c649970167cc328895c8cba7fd270=s%3A..."
> export SFMC_MC_CSRF="76be8f0de0637234bd18291..."
> export SFMC_BEARER="0073735963"
> ```

---

## Usage

### Verify credentials only
```bash
./dist/sfmc-retention ping
```

### Run all steps end-to-end
```bash
./dist/sfmc-retention run \
  --business-unit-id BU_123 \
  --output-dir ./output \
  --verbose
```

### Run isolated steps
```bash
# step1: API -> CSV + DB
./dist/sfmc-retention step-fetch-journeys --business-unit-id BU_123 --run-id run_20260430 --output-dir ./output

# step2: from step1 DB data
./dist/sfmc-retention step-resolve-event-defs --business-unit-id BU_123 --run-id run_20260430

# step3: from step2 DB data
./dist/sfmc-retention step-fetch-data-extensions --business-unit-id BU_123 --run-id run_20260430

# step4: from step3 DB data
./dist/sfmc-retention step-update-retention --business-unit-id BU_123 --run-id run_20260430

# step5: from step4 DB data
./dist/sfmc-retention step-refetch-updated --business-unit-id BU_123 --run-id run_20260430
```

### Run DB migration only
```bash
./dist/sfmc-retention migrate
```

### Output files (in `--output-dir`)
| File | Contents |
|---|---|
| `step1_journey_refs_YYYYMMDD_HHMMSS.csv` | Journey + trigger event definition references |
| `step2_event_defs_YYYYMMDD_HHMMSS.csv` | Event definition to data extension mapping |
| `step3_data_extensions_YYYYMMDD_HHMMSS.csv` | Data extension snapshot before update (input for update step) |
| `step4_update_results_YYYYMMDD_HHMMSS.csv` | Per-DE update status (ok/error) |
| `step5_after_YYYYMMDD_HHMMSS.csv` | Updated DEs re-fetched after PATCH |

---

## Business Unit Identifier

- Required for all step commands.
- Resolution order:
  1. `--business-unit-id`
  2. `SFMC_BUSINESS_UNIT_ID` from env / `.env`

---

## PostgreSQL Persistence

Set these env vars (also available in `.env.example`):

```bash
SFMC_DB_HOST=127.0.0.1
SFMC_DB_PORT=5432
SFMC_DB_NAME=sfmc_retention
SFMC_DB_USER=postgres
SFMC_DB_PASSWORD=
SFMC_DB_SSLMODE=disable
```

Each command auto-creates schema if missing and persists rows to:
- `journey_refs`
- `event_defs`
- `data_extensions`
- `update_results`
- `after_data_extensions`

Step inputs are loaded from DB only. On `step-resolve-event-defs` through `step-refetch-updated`, the command reads prior-step rows by `business_unit_id` + `run_id`. If `run-id` is omitted, latest run for that BU is used.

---

## Common Flags

- `--business-unit-id`: BU identifier (required unless `SFMC_BUSINESS_UNIT_ID` set)
- `--run-id`: logical run key for DB reads/writes and CSV naming
- `--output-dir`: output CSV folder (default `./output`)
- `--dry-run`: skip retention update call (step4/run)
- `--verbose`: print detailed progress
- `--debug`: print raw request/response logs

---

## Re-authentication

If credentials expire mid-run (HTTP 401/403), the app will:
1. Print an error message
2. Prompt you to re-enter credentials interactively (up to 3 attempts)
3. Continue step execution after re-verification

---

## Architecture

```
cmd/main.go               ← CLI (cobra), orchestration
internal/
  client/client.go        ← All HTTP calls to SFMC APIs
  models/models.go        ← Structs for Journey, EventDef, DataExtension
  worker/worker.go        ← Concurrent fan-out pipelines
  exporter/csv.go         ← Step CSV export + import
  store/*.go              ← Postgres config, schema, repository
  ping/ping.go            ← Auth verify + interactive re-prompt
```

Concurrency limits (tunable in `worker.go`):
- Journey detail fetching: 10 concurrent
- Event definition fetching: 10 concurrent  
- Data extension fetching: 10 concurrent
- DE updates (PATCH): 5 concurrent (conservative to avoid rate limits)
