#!/usr/bin/env python3
"""
main.py
────────────────────
Durable, resumable Marketing Cloud contact export backed by PostgreSQL.

Commands
--------
  init   Create a fetch_run and enqueue all page jobs (no HTTP calls).
  run    Process pending/failed jobs from the queue, write CSV, resume safely.
  status Show progress of a run.

Flow
----
  1. `init`  → inserts 1 fetch_run row + N fetch_job rows (status=pending).
  2. `run`   → workers claim jobs (status=in_progress), fetch, mark done/failed.
  3. On 403  → all workers pause, CLI prompts for new tokens, retry same page.
  4. Ctrl+C  → in_progress jobs are reset to pending on next `run` invocation.
  5. `run`   again → picks up where it left off (pending + previously failed).
"""

import argparse
import csv
import math
import os
import re
import signal
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from datetime import datetime, timezone

import psycopg2
import psycopg2.extras
import requests
from dotenv import load_dotenv
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry
from tqdm import tqdm

load_dotenv()

# ─────────────────────────────────────────────────────────────────────────────
# Config from env
# ─────────────────────────────────────────────────────────────────────────────

DATABASE_URL = os.environ.get("DATABASE_URL", "")

STATIC_HEADERS = {
    "Accept": "application/json, text/javascript, */*; q=0.01",
    "Accept-Language": "en-GB,en-US;q=0.9,en;q=0.8",
    "Connection": "keep-alive",
    "Content-Type": "application/json",
    "Origin": "https://mc.s12.marketingcloudapps.com",
    "Referer": "https://mc.s12.marketingcloudapps.com/contactsmeta/admin.html",
    "Sec-Fetch-Dest": "empty",
    "Sec-Fetch-Mode": "cors",
    "Sec-Fetch-Site": "same-origin",
    "Sec-Fetch-Storage-Access": "active",
    "User-Agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/146.0.0.0 Safari/537.36"
    ),
    "X-Requested-With": "XMLHttpRequest",
    "sec-ch-ua": '"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"',
    "sec-ch-ua-mobile": "?0",
    "sec-ch-ua-platform": '"macOS"',
    "tz": "accountPreference",
}

BASE_URL = (
    "https://mc.s12.marketingcloudapps.com"
    "/contactsmeta/fuelapi/contacts/v1/addresses/search/channel/"
)

TOTAL_RECORDS_ESTIMATE = int(os.environ.get("TOTAL_RECORDS_ESTIMATE", 19_595_920))
CHUNK_FLUSH_PAGES      = int(os.environ.get("CHUNK_FLUSH_PAGES", 500))
MAX_REAUTH_ATTEMPTS    = int(os.environ.get("MAX_REAUTH_ATTEMPTS", 5))


# ─────────────────────────────────────────────────────────────────────────────
# Database helpers
# ─────────────────────────────────────────────────────────────────────────────

def get_db_conn():
    return psycopg2.connect(DATABASE_URL)


def ensure_schema(conn):
    """Create tables if they don't exist."""
    with conn.cursor() as cur:
        cur.execute("""
        CREATE TABLE IF NOT EXISTS fetch_runs (
            id              SERIAL PRIMARY KEY,
            created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            started_at      TIMESTAMPTZ,
            finished_at     TIMESTAMPTZ,
            status          TEXT NOT NULL DEFAULT 'created',
                            -- created | running | done | failed | done_with_errors
            page_size       INT  NOT NULL,
            total_pages     INT  NOT NULL,
            filter_operator TEXT NOT NULL DEFAULT 'Is',
            output_file     TEXT NOT NULL,
            notes           TEXT
        );

        CREATE TABLE IF NOT EXISTS fetch_jobs (
            id              SERIAL PRIMARY KEY,
            run_id          INT  NOT NULL REFERENCES fetch_runs(id) ON DELETE CASCADE,
            page_number     INT  NOT NULL,
            status          TEXT NOT NULL DEFAULT 'pending',
                            -- pending | in_progress | done | failed | skipped
            attempts        INT  NOT NULL DEFAULT 0,
            items_fetched   INT,
            started_at      TIMESTAMPTZ,
            finished_at     TIMESTAMPTZ,
            error_message   TEXT,
            UNIQUE (run_id, page_number)
        );

        CREATE INDEX IF NOT EXISTS idx_fetch_jobs_run_status
            ON fetch_jobs (run_id, status);

        CREATE TABLE IF NOT EXISTS fetch_item_logs (
            id              BIGSERIAL PRIMARY KEY,
            run_id          INT  NOT NULL REFERENCES fetch_runs(id) ON DELETE CASCADE,
            job_id          INT  REFERENCES fetch_jobs(id) ON DELETE CASCADE,
            page_number     INT  NOT NULL,
            contact_key     TEXT,
            contact_key_raw BYTEA,
            fetch_status    TEXT NOT NULL,
                            -- done | failed
            error_message   TEXT,
            logged_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            error_code      TEXT,
            retry_count     INT  NOT NULL DEFAULT 0,
            is_retryable    BOOLEAN NOT NULL DEFAULT FALSE,
            next_retry_at   TIMESTAMPTZ,
            retry_status    TEXT NOT NULL DEFAULT 'none'
                            -- none | pending | in_progress | done | abandoned
        );

        CREATE UNIQUE INDEX IF NOT EXISTS uq_fetch_item_logs_done
            ON fetch_item_logs (run_id, job_id, contact_key)
            WHERE fetch_status = 'done' AND contact_key IS NOT NULL;

        CREATE UNIQUE INDEX IF NOT EXISTS uq_fetch_item_logs_done_raw
            ON fetch_item_logs (run_id, job_id, contact_key_raw)
            WHERE fetch_status = 'done' AND contact_key_raw IS NOT NULL;

        CREATE INDEX IF NOT EXISTS idx_fetch_item_logs_run_logged
            ON fetch_item_logs (run_id, logged_at);

        CREATE INDEX IF NOT EXISTS idx_fetch_item_logs_job
            ON fetch_item_logs (job_id);

        CREATE INDEX IF NOT EXISTS idx_fetch_item_logs_retry_queue
            ON fetch_item_logs (fetch_status, is_retryable, retry_status, retry_count, logged_at);

        ALTER TABLE fetch_item_logs ADD COLUMN IF NOT EXISTS error_code TEXT;
        ALTER TABLE fetch_item_logs ADD COLUMN IF NOT EXISTS contact_key_raw BYTEA;
        ALTER TABLE fetch_item_logs ADD COLUMN IF NOT EXISTS retry_count INT NOT NULL DEFAULT 0;
        ALTER TABLE fetch_item_logs ADD COLUMN IF NOT EXISTS is_retryable BOOLEAN NOT NULL DEFAULT FALSE;
        ALTER TABLE fetch_item_logs ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
        ALTER TABLE fetch_item_logs ADD COLUMN IF NOT EXISTS retry_status TEXT NOT NULL DEFAULT 'none';
        """)
    conn.commit()


# ─────────────────────────────────────────────────────────────────────────────
# Shared credentials (thread-safe)
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class AuthCredentials:
    bearer_token: str
    csrf_token:   str
    cookie:       str
    _lock: threading.RLock = None

    def __post_init__(self):
        self._lock = threading.RLock()

    def snapshot(self):
        with self._lock:
            return self.bearer_token, self.csrf_token, self.cookie

    def update(self, bearer_token: str, csrf_token: str, cookie: str):
        with self._lock:
            self.bearer_token = bearer_token
            self.csrf_token   = csrf_token
            self.cookie       = cookie


# ─────────────────────────────────────────────────────────────────────────────
# Per-thread session
# ─────────────────────────────────────────────────────────────────────────────

_thread_local = threading.local()


def _get_session(bearer: str, csrf: str, cookie: str) -> requests.Session:
    if not hasattr(_thread_local, "session"):
        adapter = HTTPAdapter(
            pool_connections=1,
            pool_maxsize=2,
            max_retries=Retry(total=0, raise_on_status=False),
        )
        s = requests.Session()
        s.mount("https://", adapter)
        _thread_local.session = s

    _thread_local.session.headers.update({
        **STATIC_HEADERS,
        "authorization": f"Bearer {bearer}",
        "X-CSRF-Token":  csrf,
        "Cookie":        cookie,
    })
    return _thread_local.session


def _invalidate_thread_session():
    if hasattr(_thread_local, "session"):
        try:
            _thread_local.session.close()
        except Exception:
            pass
        del _thread_local.session


# ─────────────────────────────────────────────────────────────────────────────
# Errors
# ─────────────────────────────────────────────────────────────────────────────

class AuthError(Exception):
    pass


class APIError(Exception):
    pass


def build_auth_guard(max_failures: int) -> dict:
    return {
        "count": 0,
        "max": max(1, max_failures),
        "lock": threading.Lock(),
        "blocked": threading.Event(),
    }


def auth_guard_record_failure(auth_guard: dict | None) -> bool:
    if auth_guard is None:
        return False
    with auth_guard["lock"]:
        auth_guard["count"] += 1
        if auth_guard["count"] >= auth_guard["max"]:
            auth_guard["blocked"].set()
            return True
    return False


def auth_guard_record_success(auth_guard: dict | None) -> None:
    if auth_guard is None:
        return
    with auth_guard["lock"]:
        auth_guard["count"] = 0


def auth_guard_force_block(auth_guard: dict | None) -> None:
    if auth_guard is None:
        return
    with auth_guard["lock"]:
        auth_guard["blocked"].set()


def build_prompt_budget(max_prompts: int) -> dict:
    return {
        "max": max(0, int(max_prompts)),
        "used": 0,
        "lock": threading.Lock(),
    }


def prompt_budget_can_prompt(budget: dict | None) -> bool:
    if budget is None:
        return True
    with budget["lock"]:
        return budget["used"] < budget["max"]


def prompt_budget_record_consumed(budget: dict | None) -> None:
    if budget is None:
        return
    with budget["lock"]:
        budget["used"] += 1


def sanitize_db_text(value: str | None) -> str | None:
    if value is None:
        return None
    return value.replace("\x00", "")


def contact_key_storage(value: str | None) -> tuple[str | None, bytes | None]:
    if value is None:
        return None, None
    raw = value.encode("utf-8", errors="surrogatepass")
    # PostgreSQL TEXT cannot store NUL bytes.
    if "\x00" in value:
        return None, raw
    return value, raw


def classify_error(error: str | None) -> tuple[str, bool]:
    if not error:
        return "UNKNOWN", False

    msg = error.lower()
    code_match = re.search(r"\b(\d{3})\b", msg)
    http_code = int(code_match.group(1)) if code_match else None

    if http_code == 429:
        return "HTTP_429", True
    if http_code and 500 <= http_code <= 599:
        return f"HTTP_{http_code}", True
    if "timeout" in msg or "timed out" in msg:
        return "TIMEOUT", True
    if any(s in msg for s in ("connection", "dns", "reset by peer", "temporarily unavailable")):
        return "NETWORK", True
    if http_code in (401, 403) or "auth" in msg or "forbidden" in msg or "unauthorized" in msg:
        return f"AUTH_{http_code}" if http_code else "AUTH", False
    if "item-log insert failed" in msg or "nul" in msg or "string literal cannot contain" in msg:
        return "INTERNAL_DB_WRITE", False

    return "APP_ERROR", False


# ─────────────────────────────────────────────────────────────────────────────
# Payload parsing
# ─────────────────────────────────────────────────────────────────────────────

def _check_payload(payload, status_code: int) -> None:
    if isinstance(payload, list):
        return
    if status_code in (401, 403):
        msg = (payload.get("message") or payload.get("error_description")
               or f"HTTP {status_code}")
        raise AuthError(f"Authentication failed: {msg}")
    error_flag    = payload.get("hasErrors") or payload.get("status") == "ERROR"
    error_message = (payload.get("message") or payload.get("errorMessage")
                     or payload.get("error"))
    if error_flag or (isinstance(error_message, str) and error_message):
        lower = (error_message or "").lower()
        if any(w in lower for w in ("unauthorized", "unauthenticated",
                                    "invalid token", "access denied",
                                    "forbidden", "session")):
            raise AuthError(f"Authentication failed: {error_message}")
        raise APIError(f"API error: {error_message or payload}")


def _extract_items(payload) -> list:
    if isinstance(payload, list):
        return payload
    return (
        payload.get("items")
        or payload.get("addresses")
        or payload.get("data")
        or payload.get("results")
        or []
    )


# ─────────────────────────────────────────────────────────────────────────────
# Re-auth prompt (serialised — only one thread prompts at a time)
# ─────────────────────────────────────────────────────────────────────────────

_reauth_lock        = threading.Lock()
_reauth_done        = threading.Event()
_reauth_in_progress = False


def _normalize_bearer_input(value: str) -> str:
    token = value.strip().strip("\"' ")
    if token.lower().startswith("bearer "):
        token = token[7:].strip()
    return token


def _normalize_csrf_input(value: str) -> str:
    return value.strip().strip("\"' ")


def _normalize_cookie_input(value: str) -> str:
    cookie = value.strip().strip("\"' ")
    # Users often paste with line breaks from devtools; HTTP Cookie must be one line.
    cookie = " ".join(cookie.splitlines())
    return cookie


def fetch_page_once_no_prompt(
    page: int,
    page_size: int,
    filter_operator: str,
    creds: AuthCredentials,
    retries: int = 3,
    backoff: float = 1.5,
) -> tuple[list | None, str | None, bool]:
    """
    Single-page POST without interactive re-auth.
    Returns (items, None, False) on success.
    Returns (None, error_message, is_auth_error) on failure.
    """
    bearer, csrf, cookie = creds.snapshot()
    session = _get_session(bearer, csrf, cookie)
    params = {"$pageSize": page_size, "$page": page, "$orderBy": "contactKey ASC"}
    body = {"filterConditionOperator": filter_operator}

    for attempt in range(1, retries + 1):
        try:
            resp = session.post(BASE_URL, params=params, json=body, timeout=30)
            try:
                payload = resp.json()
            except Exception:
                payload = {}

            if not resp.ok:
                _check_payload(payload, resp.status_code)
                resp.raise_for_status()

            _check_payload(payload, resp.status_code)
            return _extract_items(payload), None, False

        except AuthError as exc:
            return None, str(exc), True

        except APIError as exc:
            return None, str(exc), False

        except requests.RequestException as exc:
            if attempt == retries:
                return None, str(exc), False
            time.sleep(backoff * attempt)

    return None, "exceeded retries", False


def ensure_auth_with_page_one_ping(
    creds: AuthCredentials,
    page_size: int,
    filter_operator: str,
    stop_event: threading.Event,
    prompt_budget: dict | None,
    auth_guard: dict | None,
    pbar=None,
) -> bool:
    """
    Ping page 1 until success or credential prompt budget exhausted.
    Returns True if authenticated; False to abort run (caller should exit).
    """
    while True:
        if stop_event.is_set():
            return False
        items, err, is_auth = fetch_page_once_no_prompt(
            1, page_size, filter_operator, creds,
        )
        if err is None:
            auth_guard_record_success(auth_guard)
            print("✅  Auth OK (ping page 1)")
            return True

        if not is_auth:
            print(f"❌  Auth ping failed (non-auth): {err}")
            return False

        if not prompt_budget_can_prompt(prompt_budget):
            print(
                "❌  Authentication still failing after maximum credential prompts "
                f"({prompt_budget['max'] if prompt_budget else 0}). Exiting."
            )
            return False

        _invalidate_thread_session()
        ok = prompt_for_new_credentials(
            creds, pbar, stop_event=stop_event, prompt_budget=prompt_budget,
        )
        if not ok:
            return False


def prompt_for_new_credentials(
    creds: AuthCredentials,
    pbar=None,
    stop_event: threading.Event | None = None,
    prompt_budget: dict | None = None,
) -> bool:
    global _reauth_in_progress

    with _reauth_lock:
        if stop_event is not None and stop_event.is_set():
            _reauth_done.set()
            return False

        if _reauth_in_progress:
            pass   # another thread is driving — fall through to wait below
        else:
            _reauth_in_progress = True
            _reauth_done.clear()

            if pbar is not None:
                pbar.clear()

            print(
                "\n\n🔐  Authentication expired (HTTP 401/403).\n"
                "    Open the Network tab in your browser, trigger any MC\n"
                "    request, and paste the three values below.\n"
            )

            while True:
                if stop_event is not None and stop_event.is_set():
                    _reauth_in_progress = False
                    _reauth_done.set()
                    return False
                try:
                    new_bearer_raw = input("   New Bearer token  : ")
                    new_csrf_raw   = input("   New CSRF token    : ")
                    new_cookie_raw = input("   New Cookie string : ")
                except (KeyboardInterrupt, EOFError):
                    print("\n⚠️   Re-auth input interrupted.")
                    _reauth_in_progress = False
                    _reauth_done.set()
                    return False

                new_bearer = _normalize_bearer_input(new_bearer_raw)
                new_csrf   = _normalize_csrf_input(new_csrf_raw)
                new_cookie = _normalize_cookie_input(new_cookie_raw)
                if new_bearer and new_csrf and new_cookie:
                    break
                print("   ⚠  All three values are required — please try again.\n")

            creds.update(new_bearer, new_csrf, new_cookie)
            prompt_budget_record_consumed(prompt_budget)
            print("✅  Credentials updated — resuming fetch…\n")

            _reauth_in_progress = False
            _reauth_done.set()
            return True

    # Another thread was prompting — wait until it finishes.
    while not _reauth_done.wait(timeout=0.25):
        if stop_event is not None and stop_event.is_set():
            return False
    # Waiter did not consume a prompt slot; driver thread recorded consumption.
    return True


# ─────────────────────────────────────────────────────────────────────────────
# Single-page HTTP fetch
# ─────────────────────────────────────────────────────────────────────────────

def fetch_page_http(
    page: int,
    page_size: int,
    filter_operator: str,
    creds: AuthCredentials,
    retries: int = 4,
    backoff: float = 1.5,
    pbar=None,
    stop_event: threading.Event | None = None,
    auth_guard: dict | None = None,
    prompt_budget: dict | None = None,
) -> tuple[list, str | None]:
    """
    Returns (items, error_message).
    error_message is None on success.
    Handles re-auth internally — will block and prompt if a 403 is received,
    then retry up to MAX_REAUTH_ATTEMPTS times before giving up.
    """
    reauth_attempts = 0

    while True:
        if stop_event is not None and stop_event.is_set():
            return [], "interrupted"
        if auth_guard is not None and auth_guard["blocked"].is_set():
            return [], "authentication blocked: too many auth failures"

        bearer, csrf, cookie = creds.snapshot()
        session = _get_session(bearer, csrf, cookie)
        params  = {"$pageSize": page_size, "$page": page, "$orderBy": "contactKey ASC"}
        body    = {"filterConditionOperator": filter_operator}

        for attempt in range(1, retries + 1):
            try:
                resp = session.post(BASE_URL, params=params, json=body, timeout=30)
                try:
                    payload = resp.json()
                except Exception:
                    payload = {}

                if not resp.ok:
                    _check_payload(payload, resp.status_code)
                    resp.raise_for_status()

                _check_payload(payload, resp.status_code)
                auth_guard_record_success(auth_guard)
                return _extract_items(payload), None

            except AuthError as exc:
                if stop_event is not None and stop_event.is_set():
                    return [], "interrupted"
                if prompt_budget is not None and not prompt_budget_can_prompt(prompt_budget):
                    auth_guard_force_block(auth_guard)
                    if stop_event is not None:
                        stop_event.set()
                    return [], (
                        "authentication failed: credential prompt limit reached"
                    )
                if prompt_budget is None:
                    if auth_guard_record_failure(auth_guard):
                        if stop_event is not None:
                            stop_event.set()
                        return [], (
                            "authentication blocked: reached global auth failure "
                            "threshold"
                        )
                if reauth_attempts >= MAX_REAUTH_ATTEMPTS:
                    return [], (
                        f"{exc} — gave up after {MAX_REAUTH_ATTEMPTS} re-auth attempts"
                    )
                reauth_attempts += 1
                _invalidate_thread_session()
                ok = prompt_for_new_credentials(
                    creds, pbar, stop_event=stop_event, prompt_budget=prompt_budget,
                )
                if not ok:
                    return [], "interrupted"
                break   # re-read creds and retry from outer while

            except APIError as exc:
                return [], str(exc)

            except requests.RequestException as exc:
                if attempt == retries:
                    return [], str(exc)
                time.sleep(backoff * attempt)

    return [], "exceeded retries"


# ─────────────────────────────────────────────────────────────────────────────
# Flatten
# ─────────────────────────────────────────────────────────────────────────────

def flatten_item(item: dict) -> dict:
    row: dict = {}

    def _f(obj, prefix=""):
        if isinstance(obj, dict):
            for k, v in obj.items():
                _f(v, f"{prefix}{k}.")
        elif isinstance(obj, list):
            for i, v in enumerate(obj):
                _f(v, f"{prefix}{i}.")
        else:
            row[prefix.rstrip(".")] = obj

    _f(item)
    return row


# ─────────────────────────────────────────────────────────────────────────────
# Streaming CSV writer
# ─────────────────────────────────────────────────────────────────────────────

class StreamingCSVWriter:
    def __init__(self, path: str):
        self.path        = path
        self._fh         = None
        self._writer     = None
        self._fieldnames: list[str] = []
        self._lock       = threading.Lock()

    def flush(self, rows: list[dict]) -> None:
        if not rows:
            return
        with self._lock:
            new_keys = [k for row in rows for k in row
                        if k not in self._fieldnames]
            self._fieldnames.extend(dict.fromkeys(new_keys))

            if self._fh is None:
                self._fh    = open(self.path, "a", newline="", encoding="utf-8")
                self._writer = csv.DictWriter(
                    self._fh, fieldnames=self._fieldnames, extrasaction="ignore"
                )
                # Write header only if file is new / empty
                if self._fh.tell() == 0:
                    self._writer.writeheader()
            elif new_keys:
                print(
                    f"\n⚠   New CSV columns discovered: {new_keys} "
                    "(empty in earlier rows)",
                    file=sys.stderr,
                )

            self._writer.writerows(rows)
            self._fh.flush()

    def close(self):
        with self._lock:
            if self._fh:
                self._fh.close()
                self._fh = None


# ─────────────────────────────────────────────────────────────────────────────
# DB job helpers
# ─────────────────────────────────────────────────────────────────────────────

def db_claim_job(run_id: int, conn=None) -> dict | None:
    """
    Atomically claim one pending job for this worker.
    Returns the job row dict, or None if no jobs remain.
    """
    own_conn = conn is None
    conn = conn or get_db_conn()
    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute("""
                UPDATE fetch_jobs
                SET    status     = 'in_progress',
                       started_at = NOW(),
                       attempts   = attempts + 1
                WHERE  id = (
                    SELECT id FROM fetch_jobs
                    WHERE  run_id = %s
                      AND  status IN ('pending', 'failed')
                    ORDER BY page_number
                    FOR UPDATE SKIP LOCKED
                    LIMIT 1
                )
                RETURNING *
            """, (run_id,))
            row = cur.fetchone()
            conn.commit()
            return dict(row) if row else None
    finally:
        if own_conn:
            conn.close()


def db_mark_done(job_id: int, items_fetched: int, conn=None):
    own_conn = conn is None
    conn = conn or get_db_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                UPDATE fetch_jobs
                SET status = 'done',
                    finished_at   = NOW(),
                    items_fetched = %s,
                    error_message = NULL
                WHERE id = %s
            """, (items_fetched, job_id))
        conn.commit()
    finally:
        if own_conn:
            conn.close()


def db_mark_failed(job_id: int, error: str, conn=None):
    own_conn = conn is None
    conn = conn or get_db_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                UPDATE fetch_jobs
                SET status        = 'failed',
                    finished_at   = NOW(),
                    error_message = %s
                WHERE id = %s
            """, (error[:2000], job_id))
        conn.commit()
    finally:
        if own_conn:
            conn.close()


def db_reset_in_progress(run_id: int):
    """Reset stale in_progress jobs back to pending (e.g. after crash/Ctrl+C)."""
    conn = get_db_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                UPDATE fetch_jobs
                SET status = 'pending', started_at = NULL
                WHERE run_id = %s AND status = 'in_progress'
            """, (run_id,))
            n = cur.rowcount
        conn.commit()
        if n:
            print(f"♻️   Reset {n} stale in_progress job(s) → pending")
    finally:
        conn.close()


def db_run_status(run_id: int) -> dict:
    conn = get_db_conn()
    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute("SELECT * FROM fetch_runs WHERE id = %s", (run_id,))
            run = dict(cur.fetchone())
            cur.execute("""
                SELECT status, COUNT(*) AS cnt, COALESCE(SUM(items_fetched),0) AS items
                FROM   fetch_jobs
                WHERE  run_id = %s
                GROUP  BY status
            """, (run_id,))
            jobs = {r["status"]: {"count": r["cnt"], "items": r["items"]}
                    for r in cur.fetchall()}
        return {"run": run, "jobs": jobs}
    finally:
        conn.close()


def db_retry_status(run_id: int) -> dict:
    conn = get_db_conn()
    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute("""
                SELECT retry_status, COUNT(*) AS cnt
                FROM fetch_item_logs
                WHERE run_id = %s AND fetch_status = 'failed'
                GROUP BY retry_status
            """, (run_id,))
            return {r["retry_status"]: int(r["cnt"]) for r in cur.fetchall()}
    finally:
        conn.close()


def db_list_incomplete_runs() -> list[int]:
    conn = get_db_conn()
    try:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT id
                FROM fetch_runs
                WHERE status IN ('created', 'running', 'failed', 'done_with_errors')
                ORDER BY created_at ASC, id ASC
            """)
            rows = cur.fetchall()
            return [int(row[0]) for row in rows]
    finally:
        conn.close()


def _calc_adaptive_workers(
    remaining_units: int,
    configured_workers: int,
    adaptive_enabled: bool,
    min_workers: int,
    max_workers: int,
) -> int:
    if not adaptive_enabled:
        return max(1, configured_workers)

    cpu = os.cpu_count() or 4
    min_w = max(1, min_workers)
    max_w = max(min_w, max_workers)
    # IO-heavy workload: allow oversubscription relative to CPU.
    cpu_based_cap = max(min_w, cpu * 4)
    cap = min(max_w, cpu_based_cap)
    return max(min_w, min(cap, max(1, remaining_units)))


def extract_contact_key(item: dict) -> str | None:
    contact_key = item.get("contactKey")
    if isinstance(contact_key, dict):
        value = contact_key.get("value")
    else:
        value = contact_key

    if value is None:
        return None

    return str(value)


def db_complete_job_success(
    run_id: int,
    job_id: int,
    page_number: int,
    items: list[dict],
    conn,
):
    """Persist item logs and mark job done in one transaction."""
    rows = []
    for item in items:
        key_text, key_raw = contact_key_storage(extract_contact_key(item))
        rows.append((
            run_id,
            job_id,
            page_number,
            key_text,
            psycopg2.Binary(key_raw) if key_raw is not None else None,
            "done",
            None,
        ))
    with conn.cursor() as cur:
        if rows:
            psycopg2.extras.execute_values(
                cur,
                """
                INSERT INTO fetch_item_logs
                    (run_id, job_id, page_number, contact_key, contact_key_raw, fetch_status, error_message)
                VALUES %s
                ON CONFLICT DO NOTHING
                """,
                rows,
                page_size=5000,
            )
        cur.execute(
            """
            UPDATE fetch_jobs
            SET status = 'done',
                finished_at   = NOW(),
                items_fetched = %s,
                error_message = NULL
            WHERE id = %s
            """,
            (len(items), job_id),
        )
    conn.commit()


def db_complete_job_failure(
    run_id: int,
    job_id: int,
    page_number: int,
    error: str,
    conn,
):
    """Persist failure log and mark job failed in one transaction."""
    err = sanitize_db_text(error)[:2000]
    error_code, is_retryable = classify_error(err)
    retry_status = "pending" if is_retryable else "abandoned"
    with conn.cursor() as cur:
        cur.execute(
            """
            UPDATE fetch_jobs
            SET status        = 'failed',
                finished_at   = NOW(),
                error_message = %s
            WHERE id = %s
            """,
            (err, job_id),
        )
        cur.execute(
            """
            INSERT INTO fetch_item_logs
                (run_id, job_id, page_number, contact_key, fetch_status, error_message,
                 error_code, is_retryable, retry_status)
            VALUES (%s, %s, %s, NULL, 'failed', %s, %s, %s, %s)
            """,
            (run_id, job_id, page_number, err, error_code, is_retryable, retry_status),
        )
    conn.commit()


def db_claim_retry_item(run_id: int, max_attempts: int, conn) -> dict | None:
    with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
        cur.execute("""
            UPDATE fetch_item_logs
            SET retry_status = 'in_progress'
            WHERE id = (
                SELECT id
                FROM fetch_item_logs
                WHERE run_id = %s
                  AND fetch_status = 'failed'
                  AND is_retryable = TRUE
                  AND retry_count < %s
                  AND retry_status IN ('none', 'pending')
                  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
                ORDER BY logged_at ASC
                FOR UPDATE SKIP LOCKED
                LIMIT 1
            )
            RETURNING *
        """, (run_id, max_attempts))
        row = cur.fetchone()
    conn.commit()
    return dict(row) if row else None


def db_mark_retry_done(log_id: int, conn):
    with conn.cursor() as cur:
        cur.execute("""
            UPDATE fetch_item_logs
            SET retry_status = 'done',
                error_message = NULL,
                next_retry_at = NULL
            WHERE id = %s
        """, (log_id,))
    conn.commit()


def db_mark_retry_result(log_id: int, retry_count: int, error: str, max_attempts: int, conn):
    err = sanitize_db_text(error or "retry failed")[:2000]
    error_code, is_retryable = classify_error(err)
    next_count = retry_count + 1
    retry_status = "pending" if (is_retryable and next_count < max_attempts) else "abandoned"

    with conn.cursor() as cur:
        cur.execute("""
            UPDATE fetch_item_logs
            SET retry_count   = %s,
                error_message = %s,
                error_code    = %s,
                is_retryable  = %s,
                retry_status  = %s,
                next_retry_at = CASE
                    WHEN %s = 'pending' THEN NOW() + INTERVAL '30 seconds'
                    ELSE NULL
                END
            WHERE id = %s
        """, (next_count, err, error_code, is_retryable, retry_status, retry_status, log_id))
    conn.commit()


# ─────────────────────────────────────────────────────────────────────────────
# COMMAND: init
# ─────────────────────────────────────────────────────────────────────────────

def cmd_init(args) -> int:
    """Create a new fetch_run and enqueue all page jobs. No HTTP calls."""
    conn = get_db_conn()
    ensure_schema(conn)

    total_pages = math.ceil(TOTAL_RECORDS_ESTIMATE / args.page_size)

    with conn.cursor() as cur:
        cur.execute("""
            INSERT INTO fetch_runs
                (page_size, total_pages, filter_operator, output_file, notes)
            VALUES (%s, %s, %s, %s, %s)
            RETURNING id
        """, (
            args.page_size,
            total_pages,
            args.filter_operator,
            args.output,
            args.notes or None,
        ))
        run_id = cur.fetchone()[0]

        # Bulk-insert all jobs
        psycopg2.extras.execute_values(
            cur,
            "INSERT INTO fetch_jobs (run_id, page_number) VALUES %s",
            [(run_id, p) for p in range(1, total_pages + 1)],
            page_size=5000,
        )
    conn.commit()
    conn.close()

    print(
        f"\n✅  Run #{run_id} created\n"
        f"    Pages queued : {total_pages:,}\n"
        f"    Page size    : {args.page_size}\n"
        f"    Output file  : {args.output}\n"
        f"\nRun it with:\n"
        f"  python main-persisted.py run --run-id {run_id} "
        f"--bearer-token <TOKEN> --csrf-token <TOKEN> --cookie <COOKIE>\n"
    )
    return run_id


# ─────────────────────────────────────────────────────────────────────────────
# COMMAND: run
# ─────────────────────────────────────────────────────────────────────────────

def _run_single(args, run_id: int):
    conn = get_db_conn()
    ensure_schema(conn)

    # Load run metadata
    with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
        cur.execute("SELECT * FROM fetch_runs WHERE id = %s", (run_id,))
        run = cur.fetchone()
    conn.close()

    if run is None:
        print(f"❌  No run found with id={run_id}")
        return 1

    page_size       = run["page_size"]
    filter_operator = run["filter_operator"]
    output_file     = run["output_file"]

    # Reset any stale in_progress from a previous crash
    db_reset_in_progress(run_id)

    # Mark run as running
    conn = get_db_conn()
    with conn.cursor() as cur:
        cur.execute("""
            UPDATE fetch_runs
            SET status = 'running', started_at = COALESCE(started_at, NOW())
            WHERE id = %s
        """, (run_id,))
    conn.commit()
    conn.close()

    creds = AuthCredentials(
        bearer_token=args.bearer_token,
        csrf_token=args.csrf_token,
        cookie=args.cookie,
    )

    stop_event = threading.Event()
    interrupted = threading.Event()
    auth_guard = build_auth_guard(args.max_auth_failures)
    prompt_budget = build_prompt_budget(args.max_auth_prompts)

    def _sigint_handler(sig, frame):
        print("\n\n⚠️   Interrupted — cleaning up…")
        interrupted.set()
        stop_event.set()

    signal.signal(signal.SIGINT, _sigint_handler)

    if not ensure_auth_with_page_one_ping(
        creds,
        page_size,
        filter_operator,
        stop_event,
        prompt_budget,
        auth_guard,
        pbar=None,
    ):
        if not stop_event.is_set():
            print("❌  Exiting: authentication failed or credential prompt limit reached.")
        return 1

    csv_writer = StreamingCSVWriter(output_file)

    # ── tqdm ──────────────────────────────────────────────────────────────
    status_snapshot = db_run_status(run_id)["jobs"]
    done_count      = int(status_snapshot.get("done", {}).get("count", 0))
    failed_count    = int(status_snapshot.get("failed", {}).get("count", 0))
    pending_count   = int(status_snapshot.get("pending", {}).get("count", 0))
    total_pages     = run["total_pages"]
    remaining_jobs  = pending_count + failed_count
    worker_count    = _calc_adaptive_workers(
        remaining_units=remaining_jobs,
        configured_workers=args.workers,
        adaptive_enabled=args.adaptive_workers,
        min_workers=args.min_workers,
        max_workers=args.max_workers,
    )
    print(
        f"⚙️   Worker count: {worker_count} "
        f"(adaptive={'on' if args.adaptive_workers else 'off'}, remaining_jobs={remaining_jobs})"
    )

    pbar = tqdm(
        total=total_pages,
        initial=done_count,
        desc=f"Run #{run_id}",
        unit="pg",
        colour="cyan",
        dynamic_ncols=True,
    )

    total_records = 0
    records_lock  = threading.Lock()

    # ── writer thread ─────────────────────────────────────────────────────
    pending_rows: list[dict] = []
    pending_lock  = threading.Lock()

    def writer_thread():
        while not stop_event.is_set():
            time.sleep(0.5)
            _flush_pending()
        _flush_pending()   # final drain

    def _flush_pending():
        with pending_lock:
            if not pending_rows:
                return
            batch = pending_rows[:]
            pending_rows.clear()
        csv_writer.flush(batch)

    wt = threading.Thread(target=writer_thread, daemon=True)
    wt.start()

    # ── worker ────────────────────────────────────────────────────────────
    def worker():
        nonlocal total_records
        worker_conn = get_db_conn()
        try:
            while not stop_event.is_set():
                job = db_claim_job(run_id, conn=worker_conn)
                if job is None:
                    break   # queue exhausted

                page    = job["page_number"]
                job_id  = job["id"]

                items, error = fetch_page_http(
                    page, page_size, filter_operator,
                    creds,
                    retries=args.retries,
                    pbar=pbar,
                    stop_event=stop_event,
                    auth_guard=auth_guard,
                    prompt_budget=prompt_budget,
                )

                if error:
                    db_complete_job_failure(run_id, job_id, page, error, worker_conn)
                    if auth_guard["blocked"].is_set():
                        stop_event.set()
                    pbar.update(1)
                    pbar.set_postfix({"❌ pg": page, "err": error[:40]}, refresh=True)
                    continue

                # Empty page = end of data — mark skipped, stop workers
                if not items:
                    db_mark_done(job_id, 0, conn=worker_conn)
                    pbar.update(1)
                    stop_event.set()
                    break

                flat = [flatten_item(it) for it in items]

                try:
                    # Keep job completion atomic with per-item log persistence.
                    db_complete_job_success(run_id, job_id, page, items, worker_conn)
                except Exception as exc:
                    db_complete_job_failure(
                        run_id,
                        job_id,
                        page,
                        f"item-log insert failed: {exc}",
                        worker_conn,
                    )
                    pbar.update(1)
                    pbar.set_postfix({"❌ pg": page, "err": "item-log insert failed"}, refresh=True)
                    continue

                with pending_lock:
                    pending_rows.extend(flat)

                pbar.update(1)

                with records_lock:
                    total_records += len(items)
                pbar.set_postfix({"rec": total_records, "pg": page}, refresh=True)
        finally:
            worker_conn.close()

    # ── thread pool ───────────────────────────────────────────────────────
    try:
        with ThreadPoolExecutor(max_workers=worker_count) as executor:
            futures = [executor.submit(worker) for _ in range(worker_count)]
            for f in as_completed(futures):
                try:
                    f.result()
                except Exception as exc:
                    print(f"\n⚠  Worker exception: {exc}", file=sys.stderr)
    finally:
        pbar.close()
        stop_event.set()
        wt.join(timeout=60)
        _flush_pending()
        csv_writer.close()

    # ── finalize run status ───────────────────────────────────────────────
    db_reset_in_progress(run_id)   # clean up any that didn't finish

    snap        = db_run_status(run_id)["jobs"]
    failed      = int(snap.get("failed", {}).get("count", 0))
    in_progress = int(snap.get("in_progress", {}).get("count", 0))
    pending     = int(snap.get("pending", {}).get("count", 0))
    done        = int(snap.get("done", {}).get("count", 0))
    total_jobs  = run["total_pages"]
    all_done = done == total_jobs and pending == 0 and in_progress == 0

    if interrupted.is_set() or not all_done:
        final_status = "running" if (pending or in_progress) else (
            "done_with_errors" if failed else "running"
        )
        finished_at_sql = "NULL"
    else:
        final_status = "done" if failed == 0 else "done_with_errors"
        finished_at_sql = "NOW()"

    conn = get_db_conn()
    with conn.cursor() as cur:
        cur.execute(
            f"""
            UPDATE fetch_runs
            SET status = %s, finished_at = {finished_at_sql}
            WHERE id = %s
            """,
            (final_status, run_id),
        )
    conn.commit()
    conn.close()

    if final_status in ("running", "failed"):
        print(
            f"\n⚠️   Run #{run_id} paused/incomplete — {total_records:,} records so far → {output_file}\n"
            "    Re-run `run` to continue pending jobs."
        )
        if auth_guard["blocked"].is_set():
            print(
                "🛑  Stopped early: too many authentication failures. "
                "Verify bearer/csrf/cookie from one successful request and retry."
            )
    else:
        print(f"\n✅  Run #{run_id} finished — {total_records:,} records → {output_file}")
        if failed:
            print(f"⚠️   {failed} page(s) failed — re-run `run` to retry them.")
    return 0


def cmd_run(args):
    conn = get_db_conn()
    ensure_schema(conn)
    conn.close()

    if args.run_id is not None:
        return _run_single(args, args.run_id)

    run_ids = db_list_incomplete_runs()
    if not run_ids:
        print("ℹ️   No incomplete runs found.")
        return 0

    print(f"▶️   Found {len(run_ids)} incomplete run(s): {', '.join(map(str, run_ids))}")
    exit_code = 0
    for idx, run_id in enumerate(run_ids, start=1):
        print(f"\n--- Processing run {idx}/{len(run_ids)}: #{run_id} ---")
        code = _run_single(args, run_id)
        if code != 0:
            exit_code = code

    return exit_code

# ─────────────────────────────────────────────────────────────────────────────
# COMMAND: status
# ─────────────────────────────────────────────────────────────────────────────

def cmd_status(args):
    conn = get_db_conn()
    ensure_schema(conn)
    conn.close()

    data = db_run_status(args.run_id)
    run  = data["run"]
    jobs = data["jobs"]
    retry = db_retry_status(args.run_id)

    total  = run["total_pages"]
    done   = int(jobs.get("done",        {}).get("count", 0))
    failed = int(jobs.get("failed",      {}).get("count", 0))
    prog   = int(jobs.get("in_progress", {}).get("count", 0))
    pend   = int(jobs.get("pending",     {}).get("count", 0))
    items  = sum(int(v.get("items", 0)) for v in jobs.values())
    pct    = done / total * 100 if total else 0
    retry_done      = int(retry.get("done", 0))
    retry_pending   = int(retry.get("pending", 0))
    retry_abandoned = int(retry.get("abandoned", 0))
    retry_progress  = int(retry.get("in_progress", 0))

    print(f"""
┌─ Fetch Run #{run['id']} ({'─' * 30}
│  Status      : {run['status']}
│  Output      : {run['output_file']}
│  Page size   : {run['page_size']}
│  Created     : {run['created_at']}
│  Started     : {run['started_at'] or '—'}
│  Finished    : {run['finished_at'] or '—'}
├─ Progress ({'─' * 35}
│  Total pages : {total:>10,}
│  Done        : {done:>10,}   ({pct:.1f}%)
│  In-progress : {prog:>10,}
│  Pending     : {pend:>10,}
│  Failed      : {failed:>10,}
│  Records out : {items:>10,}
├─ Failed-item retries ({'─' * 24}
│  Done        : {retry_done:>10,}
│  Pending     : {retry_pending:>10,}
│  In-progress : {retry_progress:>10,}
│  Abandoned   : {retry_abandoned:>10,}
└{'─' * 45}
""")


def _retry_failed_items_single_run(args, run_id: int) -> int:
    conn = get_db_conn()
    ensure_schema(conn)
    with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
        cur.execute("SELECT * FROM fetch_runs WHERE id = %s", (run_id,))
        run = cur.fetchone()
    conn.close()

    if run is None:
        print(f"❌  No run found with id={run_id}")
        return 1

    page_size = run["page_size"]
    filter_operator = run["filter_operator"]
    max_attempts = args.max_attempts
    retry_snapshot = db_retry_status(run_id)
    retry_candidates = int(retry_snapshot.get("none", 0)) + int(retry_snapshot.get("pending", 0))
    worker_count = _calc_adaptive_workers(
        remaining_units=retry_candidates,
        configured_workers=args.workers,
        adaptive_enabled=args.adaptive_workers,
        min_workers=args.min_workers,
        max_workers=args.max_workers,
    )
    print(
        f"⚙️   Retry workers: {worker_count} "
        f"(adaptive={'on' if args.adaptive_workers else 'off'}, candidates={retry_candidates})"
    )

    creds = AuthCredentials(
        bearer_token=args.bearer_token,
        csrf_token=args.csrf_token,
        cookie=args.cookie,
    )

    stop_evt = threading.Event()
    prompt_budget_retry = build_prompt_budget(args.max_auth_prompts)
    if not ensure_auth_with_page_one_ping(
        creds,
        page_size,
        filter_operator,
        stop_evt,
        prompt_budget_retry,
        None,
        pbar=None,
    ):
        print("❌  Exiting: authentication failed or credential prompt limit reached.")
        return 1

    progress = {"done": 0, "failed": 0}
    progress_lock = threading.Lock()

    def worker():
        worker_conn = get_db_conn()
        try:
            while True:
                log_row = db_claim_retry_item(run_id, max_attempts, worker_conn)
                if log_row is None:
                    break

                page_number = int(log_row["page_number"])
                contact_key = log_row.get("contact_key")
                contact_key_raw = log_row.get("contact_key_raw")
                log_id = int(log_row["id"])
                retry_count = int(log_row.get("retry_count", 0))

                items, error = fetch_page_http(
                    page_number,
                    page_size,
                    filter_operator,
                    creds,
                    retries=args.retries,
                    stop_event=None,
                    auth_guard=None,
                    prompt_budget=prompt_budget_retry,
                )

                if error:
                    db_mark_retry_result(log_id, retry_count, error, max_attempts, worker_conn)
                    with progress_lock:
                        progress["failed"] += 1
                    continue

                if isinstance(contact_key_raw, memoryview):
                    contact_key_raw = bytes(contact_key_raw)

                if contact_key is None and contact_key_raw is None:
                    db_mark_retry_done(log_id, worker_conn)
                    with progress_lock:
                        progress["done"] += 1
                    continue

                found = False
                for item in items:
                    fetched_key = extract_contact_key(item)
                    if contact_key_raw is not None:
                        _, fetched_raw = contact_key_storage(fetched_key)
                        if fetched_raw == contact_key_raw:
                            found = True
                            break
                    else:
                        if fetched_key == contact_key:
                            found = True
                            break
                if found:
                    db_mark_retry_done(log_id, worker_conn)
                    with progress_lock:
                        progress["done"] += 1
                else:
                    db_mark_retry_result(
                        log_id,
                        retry_count,
                        f"Contact key not found on page {page_number}",
                        max_attempts,
                        worker_conn,
                    )
                    with progress_lock:
                        progress["failed"] += 1
        finally:
            worker_conn.close()

    with ThreadPoolExecutor(max_workers=worker_count) as executor:
        futures = [executor.submit(worker) for _ in range(worker_count)]
        for f in as_completed(futures):
            f.result()

    print(
        f"✅  Retry scan run #{run_id} complete "
        f"(done: {progress['done']}, failed: {progress['failed']})"
    )
    return 0


def cmd_retry_failed_items(args) -> int:
    conn = get_db_conn()
    ensure_schema(conn)
    conn.close()

    if args.run_id is not None:
        return _retry_failed_items_single_run(args, args.run_id)

    run_ids = db_list_incomplete_runs()
    if not run_ids:
        print("ℹ️   No incomplete runs found.")
        return 0

    exit_code = 0
    for run_id in run_ids:
        code = _retry_failed_items_single_run(args, run_id)
        if code != 0:
            exit_code = code
    return exit_code


# ─────────────────────────────────────────────────────────────────────────────
# CLI
# ─────────────────────────────────────────────────────────────────────────────

def parse_args():
    p = argparse.ArgumentParser(
        description="Durable MC contact export — backed by PostgreSQL.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    sub = p.add_subparsers(dest="command", required=True)

    # ── init ──────────────────────────────────────────────────────────────
    pi = sub.add_parser("init", help="Create run + enqueue all page jobs (no HTTP).")
    pi.add_argument("--page-size",       type=int, default=25, metavar="N")
    pi.add_argument("--filter-operator", default="Is",          metavar="OP")
    pi.add_argument("--output",          default="contacts.csv",metavar="FILE")
    pi.add_argument("--notes",           default="",            metavar="TEXT")

    # ── run ───────────────────────────────────────────────────────────────
    pr = sub.add_parser("run", help="Process queued jobs and write CSV.")
    pr.add_argument("--run-id",      type=int, required=False, metavar="ID")
    pr.add_argument("--bearer-token",          required=True,  metavar="TOKEN")
    pr.add_argument("--csrf-token",            required=True,  metavar="TOKEN")
    pr.add_argument("--cookie",                required=True,  metavar="COOKIE")
    pr.add_argument("--workers",     type=int, default=20,     metavar="N")
    pr.add_argument("--retries",     type=int, default=4,      metavar="N")
    pr.add_argument("--max-auth-failures", type=int, default=5, metavar="N")
    pr.add_argument("--max-auth-prompts", type=int, default=3, metavar="N")
    pr.add_argument("--adaptive-workers", action="store_true")
    pr.add_argument("--min-workers", type=int, default=4, metavar="N")
    pr.add_argument("--max-workers", type=int, default=80, metavar="N")

    # ── status ────────────────────────────────────────────────────────────
    ps = sub.add_parser("status", help="Show progress of a run.")
    ps.add_argument("--run-id", type=int, required=True, metavar="ID")

    # ── retry-failed-items ────────────────────────────────────────────────
    prf = sub.add_parser(
        "retry-failed-items",
        help="Retry failed item logs up to max attempts.",
    )
    prf.add_argument("--run-id", type=int, required=False, metavar="ID")
    prf.add_argument("--bearer-token", required=True, metavar="TOKEN")
    prf.add_argument("--csrf-token", required=True, metavar="TOKEN")
    prf.add_argument("--cookie", required=True, metavar="COOKIE")
    prf.add_argument("--workers", type=int, default=10, metavar="N")
    prf.add_argument("--retries", type=int, default=3, metavar="N")
    prf.add_argument("--max-attempts", type=int, default=3, metavar="N")
    prf.add_argument("--max-auth-prompts", type=int, default=3, metavar="N")
    prf.add_argument("--adaptive-workers", action="store_true")
    prf.add_argument("--min-workers", type=int, default=2, metavar="N")
    prf.add_argument("--max-workers", type=int, default=40, metavar="N")

    return p.parse_args()


def main():
    args = parse_args()
    if not DATABASE_URL:
        print("❌  DATABASE_URL is not set. Copy .env.example → .env and fill it in.")
        sys.exit(1)

    if args.command == "init":
        cmd_init(args)
    elif args.command == "run":
        rc = cmd_run(args)
        if rc:
            sys.exit(rc)
    elif args.command == "status":
        cmd_status(args)
    elif args.command == "retry-failed-items":
        rc = cmd_retry_failed_items(args)
        if rc:
            sys.exit(rc)


if __name__ == "__main__":
    main()
