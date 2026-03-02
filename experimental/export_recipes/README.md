# Export recipes (Evergage API)

Fetches recipe summaries from the Evergage API: first the full list, then detail for each recipe by ID.

## Requirements

- Python 3.9+
- `requests` (see [Install](#install))

## Install

```bash
pip install -r requirements.txt
```

Or with a virtualenv and Make:

```bash
make install
```

## How to run

### 1. Get cookies

Log in to the Evergage UI in your browser, open DevTools → Network, trigger a request to the same origin, then copy the **Cookie** header value (e.g. `JSESSIONID=...; AWSALBTGCORS=...`).

### 2. Set environment and run

**Export list + all details to `recipes.json`:**

```bash
export RECIPE_COOKIES='JSESSIONID=your-session-id; AWSALBTGCORS=your-alb-cookie'
python main.py -o recipes.json
```

With Make:

```bash
export RECIPE_COOKIES='JSESSIONID=...; AWSALBTGCORS=...'
make run
```

**Only fetch the list (no detail calls):**

```bash
export RECIPE_COOKIES='...'
python main.py --list-only
# or
make list-only
```

**Custom base URL:**

```bash
export RECIPE_BASE_URL='https://your-instance.evergage.com'
export RECIPE_COOKIES='...'
python main.py -o recipes.json
```

## Options

| Option | Env / default | Description |
|--------|----------------|-------------|
| `--cookies` | `RECIPE_COOKIES` | Cookie string (required). |
| `--base-url` | `RECIPE_BASE_URL` | API base URL. |
| `--time-range` | `pastWeek` | Time range for the list endpoint. |
| `-o`, `--output` | — | Write all details to this JSON file. |
| `--list-only` | — | Only fetch the list, skip detail requests. |

## Make targets

- `make help` — Show usage and env vars.
- `make install` — Create venv and install dependencies.
- `make run` — Run full export to `recipes.json` (requires `RECIPE_COOKIES`).
- `make list-only` — Run list-only export (requires `RECIPE_COOKIES`).
