#!/usr/bin/env python3
"""
add_validator.py

Read an EIP-2335 keystore JSON file and POST it to the lighthouse-launch
/createValidatorHandler endpoint ( /validator ).

Usage
=====
    python upload_validator.py /path/to/voting-keystore.json
        [--name V1] [--url http://my-server:5000]

Arguments
---------
positional:
  keystore_path        Path to the voting-keystore.json file.

optional:
  -n, --name           Validator name              (default: "V0")
  -u, --url            URL prefix *without* /validator
                       (default: "http://localhost:5000")
"""

import argparse
import json
import sys

import requests


def main() -> None:
    parser = argparse.ArgumentParser(description="Upload validator keystore")
    parser.add_argument("keystore_path", help="Path to voting-keystore.json")
    parser.add_argument(
        "-n", "--name", default="V0", help='Validator name (default: "V0")'
    )
    parser.add_argument(
        "-u",
        "--url",
        default="http://localhost:5000",
        help='Server URL prefix (default: "http://localhost:5000")',
    )
    args = parser.parse_args()

    # Load keystore JSON
    try:
        with open(args.keystore_path, "r", encoding="utf-8") as f:
            keystore_data = json.load(f)
    except Exception as exc:
        print(f"ERROR: Unable to read keystore file: {exc}", file=sys.stderr)
        sys.exit(1)

    # Build request body
    payload = {
        "name": args.name,
        "keystore": keystore_data,
    }

    endpoint = args.url.rstrip("/") + "/validator"

    try:
        resp = requests.post(endpoint, json=payload, timeout=10)
    except requests.RequestException as exc:
        print(f"ERROR: Request failed: {exc}", file=sys.stderr)
        sys.exit(1)

    # Handle response
    if resp.ok:
        print(f"Success ({resp.status_code}): {resp.text}")
    else:
        print(
            f"Server returned {resp.status_code}: {resp.text}",
            file=sys.stderr,
        )
        sys.exit(1)


if __name__ == "__main__":
    main()
