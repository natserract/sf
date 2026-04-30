#!/usr/bin/env python3
"""
Generate SFMC env values by parsing captured curl commands.

Usage:
  python3 generate_env_from_curl.py
  python3 generate_env_from_curl.py --output .env.generated
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path
from urllib.parse import urlparse


def read_text(path: Path) -> str:
    if not path.exists():
        raise FileNotFoundError(f"File not found: {path}")
    return path.read_text(encoding="utf-8")


def extract_host(curl_text: str) -> str:
    match = re.search(r"curl\s+'([^']+)'", curl_text)
    if not match:
        raise ValueError("Could not find curl URL")
    url = match.group(1)
    host = urlparse(url).hostname
    if not host:
        raise ValueError(f"Could not parse host from URL: {url}")
    return host


def extract_header(curl_text: str, header_name: str) -> str:
    pattern = rf"-H\s+'{re.escape(header_name)}:\s*([^']*)'"
    match = re.search(pattern, curl_text, flags=re.IGNORECASE)
    if not match:
        raise ValueError(f"Could not find header: {header_name}")
    value = match.group(1).strip()
    if not value:
        raise ValueError(f"Header exists but is empty: {header_name}")
    return value


def extract_header_optional(curl_text: str, header_name: str) -> str:
    try:
        return extract_header(curl_text, header_name)
    except ValueError:
        return ""


def extract_cookie(curl_text: str) -> str:
    match = re.search(r"-b\s+'([^']+)'", curl_text)
    if not match:
        raise ValueError("Could not find cookie (-b ...)")
    value = match.group(1).strip()
    if not value:
        raise ValueError("Cookie exists but is empty")
    return value


def extract_bearer(curl_text: str) -> str:
    auth = extract_header(curl_text, "authorization")
    match = re.match(r"Bearer\s+(.+)$", auth, flags=re.IGNORECASE)
    if not match:
        raise ValueError("Authorization header found, but not Bearer format")
    token = match.group(1).strip()
    if not token:
        raise ValueError("Bearer token is empty")
    return token


def parse_values(jb_text: str, mc_text: str) -> dict[str, str]:
    return {
        "SFMC_JB_HOST": extract_host(jb_text),
        "SFMC_JB_COOKIE": extract_cookie(jb_text),
        "SFMC_JB_CSRF": extract_header(jb_text, "X-CSRF-Token"),
        "SFMC_MC_HOST": extract_host(mc_text),
        "SFMC_MC_COOKIE": extract_cookie(mc_text),
        "SFMC_MC_CSRF": extract_header_optional(mc_text, "X-CSRF-Token"),
        "SFMC_BEARER": extract_bearer(mc_text),
    }


def format_env(values: dict[str, str]) -> str:
    order = [
        "SFMC_JB_HOST",
        "SFMC_JB_COOKIE",
        "SFMC_JB_CSRF",
        "SFMC_MC_HOST",
        "SFMC_MC_COOKIE",
        "SFMC_MC_CSRF",
        "SFMC_BEARER",
    ]
    return "\n".join(f"{key}={values[key]}" for key in order) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Extract SFMC env values from curl-jb.bash and curl-mc.bash."
    )
    parser.add_argument(
        "--jb-curl",
        default="curl-jb.bash",
        type=Path,
        help="Path to Journey Builder curl file (default: curl-jb.bash)",
    )
    parser.add_argument(
        "--mc-curl",
        default="curl-mc.bash",
        type=Path,
        help="Path to Marketing Cloud curl file (default: curl-mc.bash)",
    )
    parser.add_argument(
        "--output",
        type=Path,
        help="Optional output file path. If omitted, prints to stdout.",
    )
    args = parser.parse_args()

    try:
        jb_text = read_text(args.jb_curl)
        mc_text = read_text(args.mc_curl)
        values = parse_values(jb_text, mc_text)
        env_text = format_env(values)
    except (FileNotFoundError, ValueError) as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1

    if not values["SFMC_MC_CSRF"]:
        print(
            "Warning: SFMC_MC_CSRF not found in MC curl file; value left empty.",
            file=sys.stderr,
        )

    if args.output:
        args.output.write_text(env_text, encoding="utf-8")
        print(f"Wrote env values to: {args.output}")
        return 0

    print(env_text, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
