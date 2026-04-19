# fetch-all-contacts-go 

This CLI fetches all contact pages from Marketing Cloud, stores `contactKey.value` in Postgres, and tracks durable page jobs in Postgres so workers can resume safely after crashes or `CTRL+C`.

## Key Guarantees

- Durable page queue in Postgres (`pages` table).
- Distributed workers are safe (multiple processes can run at the same time).
- No duplicate page jobs (`PRIMARY KEY (run_id, page_number)`).
- No duplicate contact keys (`contact_keys.contact_key` primary key with `ON CONFLICT DO NOTHING`).
- Retry with persisted backoff (`next_attempt_at`), so retries survive restarts.
- Resumable: restarting workers continues remaining pending pages.
- Auth preflight is validated before processing starts.
- On runtime `403`, CLI asks for new bearer/csrf/cookie (shared prompt, max 3 attempts).

## How It Works

1. `start-run` validates auth and calls count endpoint to get `totalCount`.
2. Workers claim one page at a time using `FOR UPDATE SKIP LOCKED`.
3. Worker fetches API page, extracts `contactKey.value`, inserts keys idempotently.
4. Run creation pre-seeds all pages (`started-page..totalPages`) as `pending`.
5. Worker marks each page `done`/`empty`/`failed` with durable retries.
6. When no pending/in-progress pages remain, run is marked `completed`.

## Prerequisites

- Go 1.24+
- Docker (for local Postgres)
- Valid API auth values from your browser session:
  - `--bearer-token`
  - `--csrf-token`
  - `--cookie`

## Setup

```bash
cd usecases/fetch-all-contacts-go
cp .env.example .env
make docker-up
```

## Commands

### Fetch Everything (default, no run id needed)

```bash
go run . worker \
  --bearer-token "<TOKEN>" \
  --csrf-token "<CSRF>" \
  --cookie "<COOKIE>"
```

When `--run-id` is omitted, the CLI auto-creates a new run, fetches `totalCount`, pre-seeds all pages as pending, and immediately processes them until completed.

Before worker processing starts, auth is verified via preflight ping. If auth is invalid, process exits.

### Create Run

```bash
go run . start-run \
  --started-page 1 \
  --bearer-token "<TOKEN>" \
  --csrf-token "<CSRF>" \
  --cookie "<COOKIE>"
```

Output example:

```text
run_id=8f5f91d3-1d8f-4aa3-a5ca-89f8f4b00b7c total_count=19602287 total_pages=784092 seeded_rows=784092 started_page=1
```

### Start Worker (single process)

```bash
go run . worker \
  --run-id <RUN_ID> \
  --bearer-token "<TOKEN>" \
  --csrf-token "<CSRF>" \
  --cookie "<COOKIE>"
```

### Scale Workers (distributed)

Run the same worker command in multiple terminals/hosts using the same `run-id`.
Workers coordinate through Postgres row locking and will not process the same page twice.

### Check Progress

```bash
go run . status --run-id <RUN_ID>
```

### Resume

```bash
go run . resume --run-id <uuid> \
             --bearer-token … --csrf-token … --cookie
```

## Resume After Crash / CTRL+C

- Stop workers at any time.
- Start worker command again with same `--run-id`.
- Remaining pending pages and retry pages continue.
- Stale in-progress jobs are reaped back to pending using `LOCK_TIMEOUT_SECONDS`.

## Auth Failure Behavior

- Startup: invalid auth fails fast before any processing.
- Runtime `403`: process prompts for new `bearer/csrf/cookie`.
- Only one prompt flow runs per process even with many workers.
- If 3 re-auth attempts fail, worker exits.
- In non-interactive environments (no stdin), worker exits on auth failure.

## Environment Variables

- `DB_DSN`: Postgres connection string.
- `API_BASE_URL`: default `https://mc.s12.marketingcloudapps.com`.
- `PAGE_SIZE`: default `25`.
- `MAX_IN_FLIGHT`: workers per process.
- `MAX_ATTEMPTS`: retry attempts per page.
- `RETRY_INITIAL_MS`: initial retry backoff.
- `RETRY_MAX_MS`: max retry backoff.
- `LOCK_TIMEOUT_SECONDS`: reclaim stale in-progress jobs.
- `IDLE_SLEEP_MS`: worker sleep when queue is empty.
- `REAP_INTERVAL_MS`: stale lock reap interval.

## Notes

- This flow uses pre-seeded pagination: all pages are inserted to queue first for true parallel workers.
- The API may return occasional transient failures; retries are persisted in DB.
- Contact keys are trimmed before insert and stored as `bytea` for broad compatibility.

## Email Validation Web Workflow (local-first)

This project now includes a local web app to validate email values from CSV (`contactKey.value`) and compare raw-vs-cleaned values at scale.
Validation engine is powered by [`truemail-go`](https://github.com/truemail-rb/truemail-go).

### Start web app

```bash
go run . web --addr :8080 --workers 20
```

Open `http://localhost:8080/static/index.html`.
The web command wiring is defined in `cmd_web.go`.

### Workflow

1. Upload CSV with `contactID.value` and `contactKey.value` columns.
2. Backend creates a `validation_run` and stores each row in `validation_results`.
3. Worker pool validates every row through:
   - Syntax/format check (`truemail-go` regex mode)
   - Domain + DNS/MX availability check (`truemail-go` mx mode)
   - SMTP mailbox simulation (`truemail-go` smtp mode)
4. UI displays paginated/virtualized results and row detail:
   - Raw value from CSV (kept as-is)
   - Cleaned value (wrapper removal)
   - Normalized value (lowercase email used for checks)
   - Score for each test and total score
5. History is fetched lazily per selected row using `/api/history/fetch` and your bearer/csrf/cookie.

### Scoring

- Syntax: `25`
- Domain DNS: `25`
- MX: `25`
- SMTP: `25`
- Total: `0..100`

### API summary

- `POST /api/runs` (multipart `file`) -> create run + start processing
- `GET /api/runs/:id` -> progress summary
- `GET /api/runs/:id/results?offset=&limit=&q=` -> paginated rows
- `GET /api/runs/:id/results/:rowId` -> detailed row diagnostics
- `POST /api/history/fetch` -> on-demand Marketing Cloud message history fetch

### Security & operational notes

- Bearer/CSRF/Cookie are provided at history fetch time and are not required for CSV validation phases.
- SMTP checks may fail depending on network/firewall/provider policy; failure details are stored per row.
- For large CSV files, use higher worker count with caution and monitor DB/network limits.

## Notes
- The contact_key is `BYTEA` type. If you need full value, you need to cast the bytea to text
```sql
SELECT 
    encode(contact_key, 'escape') as email_text, -- Converts bytea back to readable text
    length(contact_key) as byte_len
FROM contact_keys 
WHERE contact_id = '238453796';
```