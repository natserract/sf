import json
import re
import sys
from pathlib import Path

CURL_FILE = "curl.bash"


def extract_credentials_from_curl(curl: str) -> dict:
    result = {}

    # --- Extract Cookie (from -b flag) ---
    cookie_match = re.search(r"-b\s+'([^']+)'", curl)
    if cookie_match:
        result["cookie"] = cookie_match.group(1)

    # --- Extract all -H headers ---
    header_matches = re.findall(r"-H\s+'([^']+)'", curl)
    headers = {}
    for h in header_matches:
        if ": " in h:
            key, _, value = h.partition(": ")
            headers[key.strip()] = value.strip()

    # --- Extract X-CSRF-Token ---
    for key, value in headers.items():
        if key.lower() == "x-csrf-token":
            result["csrf_token"] = value

    # --- Extract Authorization Bearer ---
    for key, value in headers.items():
        if key.lower() == "authorization":
            bearer_match = re.match(r"Bearer\s+(.+)", value, re.IGNORECASE)
            if bearer_match:
                result["authorization_bearer"] = bearer_match.group(1)
            else:
                result["authorization"] = value  # fallback: store raw value

    return result


if __name__ == "__main__":
    # Allow overriding the curl file via command-line argument
    curl_file = Path(sys.argv[1] if len(sys.argv) > 1 else CURL_FILE)

    if not curl_file.exists():
        print(f"❌ File not found: {curl_file}")
        sys.exit(1)

    curl_command = curl_file.read_text(encoding="utf-8")
    print(f"📂 Reading curl from: {curl_file}")

    credentials = extract_credentials_from_curl(curl_command)

    output_json = json.dumps(credentials, indent=2)
    print(output_json)

    # Save to credentials.json next to the input file
    output_file = curl_file.parent / "credentials.json"
    output_file.write_text(output_json, encoding="utf-8")
    print(f"\n✅ Saved to {output_file}")

    # --- Generate one-liner command ---
    bearer = credentials.get("authorization_bearer", "")
    csrf = credentials.get("csrf_token", "")
    cookie = credentials.get("cookie", "")

    one_liner = (
        f"go run -race . worker"
        f" --bearer-token {bearer}"
        f" --csrf-token {csrf}"
        f" --cookie {cookie}"
    )

    print("\n📋 One-liner command:")
    print(one_liner)
