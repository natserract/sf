#!/usr/bin/env python3
"""
Fetch recipe summaries from Evergage API: list all recipes, then fetch detail for each by id.
Requires RECIPE_COOKIES env var (e.g. JSESSIONID=...; AWSALBTGCORS=...) or --cookies.
Fetches details concurrently and writes output in batches.
"""

import argparse
import json
import os
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from urllib.parse import urljoin

try:
    import requests
except ImportError:
    print("Install requests: pip install requests", file=sys.stderr)
    sys.exit(1)
try:
    from tqdm import tqdm
except ImportError:
    print("Install tqdm: pip install tqdm", file=sys.stderr)
    sys.exit(1)

DEFAULT_BASE = "https://{{domain}}.evergage.com"
LIST_PATH = "/internal/dataset/{business_unit}/recipeSummaries/listAll"
DETAIL_PATH_TEMPLATE = "/internal/dataset/{business_unit}/recipeSummary/detail/{recipe_id}"

DEFAULT_HEADERS = {
    "accept": "application/json, text/plain, */*",
    "accept-language": "en-GB,en-US;q=0.9,en;q=0.8",
    "referer": "https://{{domain}}.evergage.com/ui/",
    "sec-ch-ua": '"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"',
    "sec-ch-ua-mobile": "?0",
    "sec-ch-ua-platform": '"macOS"',
    "sec-fetch-dest": "empty",
    "sec-fetch-mode": "cors",
    "sec-fetch-site": "same-origin",
    "sec-fetch-storage-access": "active",
    "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
    "x-requested-with": "XMLHttpRequest",
}


def get_list(base_url: str, cookies: str, time_range: str = "pastWeek") -> list:
    url = urljoin(base_url + "/", LIST_PATH.lstrip("/"))
    params = {"includeStats": "true", "timeRange": time_range}
    resp = requests.get(
        url,
        params=params,
        headers=DEFAULT_HEADERS,
        cookies=dict(_parse_cookies(cookies)),
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()


def get_detail(base_url: str, cookies: str, recipe_id: str) -> dict:
    path = DETAIL_PATH_TEMPLATE.format(recipe_id=recipe_id)
    url = urljoin(base_url + "/", path.lstrip("/"))
    resp = requests.get(
        url,
        headers=DEFAULT_HEADERS,
        cookies=dict(_parse_cookies(cookies)),
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()


def _parse_cookies(cookie_str: str) -> list[tuple[str, str]]:
    out = []
    for part in cookie_str.split(";"):
        part = part.strip()
        if not part:
            continue
        if "=" in part:
            k, v = part.split("=", 1)
            out.append((k.strip(), v.strip()))
    return out


def main() -> None:
    ap = argparse.ArgumentParser(description="Fetch recipe list and details from Evergage API")
    ap.add_argument("--base-url", default=os.environ.get("RECIPE_BASE_URL", DEFAULT_BASE), help="API base URL")
    ap.add_argument("--cookies", default=os.environ.get("RECIPE_COOKIES"), help="Cookie string (or set RECIPE_COOKIES)")
    ap.add_argument("--time-range", default="pastWeek", help="Time range for listAll (default: pastWeek)")
    ap.add_argument("-o", "--output", help="Write all details to this JSON file (default: stdout)")
    ap.add_argument("--list-only", action="store_true", help="Only fetch list, do not fetch details")
    ap.add_argument("--workers", type=int, default=8, help="Concurrent workers for detail requests (default: 8)")
    ap.add_argument("--batch-size", type=int, default=20, help="Write to file every N details (default: 20)")
    args = ap.parse_args()

    if not args.cookies:
        print("Error: set RECIPE_COOKIES or pass --cookies", file=sys.stderr)
        sys.exit(1)

    base = args.base_url.rstrip("/")
    cookies = args.cookies

    try:
        recipes = get_list(base, cookies, time_range=args.time_range)
    except requests.RequestException as e:
        print(f"List request failed: {e}", file=sys.stderr)
        if hasattr(e, "response") and e.response is not None:
            print(e.response.text[:500], file=sys.stderr)
        sys.exit(1)

    if not isinstance(recipes, list):
        print("Unexpected list response (not a list)", file=sys.stderr)
        sys.exit(1)

    if args.list_only:
        print(json.dumps(recipes, indent=2))
        return

    ids = []
    for i, summary in enumerate(recipes):
        rid = summary.get("id") if isinstance(summary, dict) else None
        if not rid:
            print(f"Skip item {i}: no id", file=sys.stderr)
            continue
        ids.append(rid)

    if not ids:
        print("No recipe ids to fetch", file=sys.stderr)
        sys.exit(1)

    details: list[dict] = []
    batch_size = args.batch_size
    out_file = open(args.output, "w") if args.output else None
    if out_file:
        out_file.write("[\n")
    first_item = True
    buffer: list[dict] = []

    def fetch_one(recipe_id: str) -> tuple[str, dict | None]:
        try:
            return recipe_id, get_detail(base, cookies, recipe_id)
        except requests.RequestException as e:
            print(f"Detail failed for id={recipe_id}: {e}", file=sys.stderr)
            return recipe_id, None

    def flush_batch(batch: list[dict]) -> None:
        nonlocal first_item
        for obj in batch:
            if not first_item:
                out_file.write(",\n")
            out_file.write("  ")
            out_file.write(json.dumps(obj, indent=2).replace("\n", "\n  "))
            first_item = False

    with ThreadPoolExecutor(max_workers=args.workers) as executor:
        futures = [executor.submit(fetch_one, rid) for rid in ids]
        with tqdm(total=len(ids), desc="Recipes", unit="recipe") as pbar:
            for future in as_completed(futures):
                _rid, detail = future.result()
                pbar.update(1)
                if detail is None:
                    continue
                if args.output:
                    buffer.append(detail)
                    while len(buffer) >= batch_size:
                        flush_batch(buffer[:batch_size])
                        buffer = buffer[batch_size:]
                else:
                    details.append(detail)

    if out_file:
        if buffer:
            flush_batch(buffer)
        out_file.write("\n]\n")
        out_file.close()
        print(f"Wrote recipe details to {args.output}", file=sys.stderr)
    else:
        print(json.dumps(details, indent=2))


if __name__ == "__main__":
    main()
