# Fetch All Contacts

Fetch all contacts and export to CSV with durable PostgreSQL-backed resume support.

## Commands

```bash
# 1) Create a run and enqueue page jobs
python main.py init --output contacts.csv

# 2a) Process one specific run
python main.py run --run-id 123 --bearer-token <TOKEN> --csrf-token <TOKEN> --cookie <COOKIE>

# 2b) Process all incomplete runs (oldest first)
python main.py run --bearer-token <TOKEN> --csrf-token <TOKEN> --cookie <COOKIE>

# 2c) Adaptive worker scaling
python main.py run --bearer-token <TOKEN> --csrf-token <TOKEN> --cookie <COOKIE> \
  --adaptive-workers --min-workers 4 --max-workers 80

# 2d) Stop after repeated auth failures (avoid endless re-auth prompts)
python main.py run --bearer-token <TOKEN> --csrf-token <TOKEN> --cookie <COOKIE> \
  --max-auth-failures 5

# 2e) Auth ping + credential prompt limit (page-1 ping before workers; default max 3 credential prompts per run)
python main.py run --bearer-token <TOKEN> --csrf-token <TOKEN> --cookie <COOKIE> \
  --max-auth-prompts 3

# 3) Inspect one run
python main.py status --run-id 123
```

Before processing jobs, `run` POSTs **page 1** as an auth ping. If auth fails, you can paste new bearer/csrf/cookie up to **`--max-auth-prompts`** times (default `3`); if page 1 still returns 401-class auth errors after that, the program exits.

`--max-auth-prompts` applies for the whole `run` (including workers). `--max-auth-failures` is still used as a circuit breaker when there is no prompt-budget path (e.g. `prompt_budget` not passed); with the default `run` flow, the prompt budget is the main limiter for interactive re-auth.

## Resume behavior

- On crash or `Ctrl+C`, jobs left in `in_progress` are reset to `pending` on next `run`.
- When `--run-id` is omitted, the runner processes every incomplete run (`created`, `running`, `failed`, `done_with_errors`) in oldest-first order.
- A paused/incomplete run keeps its run ID and can be resumed safely.

## Per-item logging

Each successful page fetch logs item-level entries into `fetch_item_logs`, including:

- `run_id`, `job_id`, `page_number`
- normalized `contact_key` (from `contactKey.value`)
- `fetch_status` and `logged_at`

Failed fetches are also logged with `fetch_status='failed'` and error details.

## Retry failed item logs

Use item-level retry processing for failed `fetch_item_logs` rows:

```bash
# Retry failed items for one run (max 3 attempts)
python main.py retry-failed-items \
  --run-id 123 \
  --bearer-token <TOKEN> \
  --csrf-token <TOKEN> \
  --cookie <COOKIE> \
  --max-attempts 3

# Retry failed items for all incomplete runs
python main.py retry-failed-items \
  --bearer-token <TOKEN> \
  --csrf-token <TOKEN> \
  --cookie <COOKIE>
```

Retry policy:
- retryable only for transient external errors (`HTTP 5xx`, `HTTP 429`, timeout/network),
- max attempts default `3`,
- non-retryable/internal failures are marked `abandoned`.
- `retry-failed-items` also performs a page-1 auth ping and honors `--max-auth-prompts` (default `3`) for the same run.

Retry command also supports adaptive workers:

```bash
python main.py retry-failed-items \
  --run-id 123 \
  --bearer-token <TOKEN> \
  --csrf-token <TOKEN> \
  --cookie <COOKIE> \
  --adaptive-workers --min-workers 2 --max-workers 40
```