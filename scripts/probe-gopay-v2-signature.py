#!/usr/bin/env python3
"""Probe GoPay 2.8.0 X-E1 v2 signing.

Default mode is offline and safe: it verifies the PR28 cipher test vector and
prints a generated X-E1 using either a captured 1867B signed_msg template or the
same synthetic fallback used by the Go client.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import hmac
import json
import os
import re
import secrets
import string
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

DEFAULT_DISPLAY_ENCODER_KEY = (
    "1V79g&FZMB#zQ9:[T+8*xr1FXYVJ#%J)LiKl?c?=JG8dc{cX?d?p-u&Ti)$<vJC"
)
DEFAULT_LEGACY_ENCODER_KEY = "4&G6DbV&j8QZs~{)(Ila_w_|v@aqJq]E-;*(J9PanZ8sm01kTi{X<iG``]d7P&L"
DEFAULT_X_E2 = "ED9A2B38749FBDE9ACA61D6A685B7"
DEFAULT_APP_VERSION = "2.8.0"
DEFAULT_APP_BUILD = "2080"
DEFAULT_APP_ID = "com.gojek.gopay"
DEFAULT_DEVICE_OS = "Android, 12"
DEFAULT_PHONE_MAKE = "samsung"
DEFAULT_PHONE_MODEL = "samsung, SM-G780F"
DEFAULT_UNIQUE_ID = "b66aedfffc4c1068"
DEFAULT_LOCATION = "35.6763787,139.649962"
DEFAULT_LOCATION_ACCURACY = "14.16100025177002"
# Keep a realistic but explicit default, because the exact x-m1 must normally be
# passed from the captured device when doing a live probe.
DEFAULT_XM1 = (
    "3:1778758474-123456,4:131072,5:samsung|3200|2,6:02:00:00:00:00:00,"
    "7:<unknown ssid>,8:1080x2148,10:1,11:00000000000000000000000000000000,"
    "15:0000000000000000,16:00000000-0000-0000-0000-000000000000"
)
DEFAULT_URL = "https://customer.gopayapi.com/v1/support/customer/activity"
DEFAULT_METHOD = "POST"
DEFAULT_BODY = "{}"
DEFAULT_NONCE = "7x4lQPoyuPdiqNmcOda0T2x2FUELObMf"
DEFAULT_TS = "1778758474793"
EXPECTED_CIPHER = "a1192922aafc7b9da815811296d60a5884dc42c4392256dbd6891f04ee9eb939"
EXPECTED_XE1 = (
    "1c163a34ccde16b8653a26f1b2c2b31e6f1dcaa9ddce6113d3761903d2904132:"
    "a1192922aafc7b9da815811296d60a5884dc42c4392256dbd6891f04ee9eb939:D:"
    "1778758474793"
)
TAIL_CONST = "c244dc56c7b6026a"
EMPTY_MD5 = "d41d8cd98f00b204e9800998ecf8427e"
TOKEN_PREFIX = b"eyJhbGciOiJkaXIi"


def hmac_sha256(key: bytes, data: bytes) -> bytes:
    return hmac.new(key, data, hashlib.sha256).digest()


def v2_cipher(nonce: str) -> tuple[str, str]:
    zero_key = b"\x00" * 64
    hkdf_data = b"\x01" * 64
    expand_tag = b"\x01" * 32
    key_c = hmac_sha256(zero_key, hkdf_data)
    key_d = hmac_sha256(key_c, hkdf_data)
    k9 = hmac_sha256(key_c, key_d + expand_tag + key_c + nonce.encode())
    t1 = hmac_sha256(k9, key_d + expand_tag)
    t2 = hmac_sha256(k9, t1 + expand_tag)
    t3 = hmac_sha256(k9, t2 + expand_tag)
    return t2.hex(), t3[:16].hex()


def hmac_inner_pad(key: bytes) -> bytes:
    if len(key) > hashlib.sha256().block_size:
        key = hashlib.sha256(key).digest()
    block = bytearray(key.ljust(hashlib.sha256().block_size, b"\x00"))
    for idx, value in enumerate(block):
        block[idx] = value ^ 0x36
    return bytes(block)


def body_md5(body: bytes) -> str:
    if not body:
        return EMPTY_MD5
    return hashlib.md5(body).hexdigest()  # nosec: protocol compatibility only


def signature_path(url: str) -> str:
    for prefix in ("https://", "http://"):
        if url.startswith(prefix):
            return url[len(prefix) :]
    return url


def clean_token(token: str) -> str:
    return token.strip().removeprefix("Bearer ").strip()


def synthetic_real_msg(args: argparse.Namespace, cipher_hex: str, t3_hex: str, body_hash: str) -> bytes:
    key = args.display_encoder_key.encode()
    token = clean_token(args.token)
    parts = [
        hmac_inner_pad(key),
        token.encode(),
        b":",
        args.phone_model.encode(),
        b":",
        args.x_m1.encode(),
        b":",
        args.app_version.encode(),
        b":",
        body_hash.encode(),
        b":",
        args.unique_id.encode(),
        b":",
        args.method.upper().encode(),
        b":",
        args.device_os.encode(),
        b":",
        str(args.ts).encode(),
        b"::",
        signature_path(args.url).encode(),
        b":",
        args.app_id.encode(),
        b":",
        cipher_hex.encode(),
        b"0000000000000000",
        b"0000000000000000",
        TAIL_CONST.encode(),
        b"0000000000000000",
        t3_hex.encode(),
    ]
    return b"".join(parts)


def last_hex_run(value: bytes, size: int) -> bytes | None:
    for idx in range(len(value) - size, -1, -1):
        chunk = value[idx : idx + size]
        if re.fullmatch(rb"[0-9a-fA-F]{" + str(size).encode() + rb"}", chunk):
            return chunk
    return None


def patch_template(template: bytes, fallback: bytes, args: argparse.Namespace, cipher_hex: str, t3_hex: str) -> bytes:
    patched = bytearray(template)
    token = clean_token(args.token).encode()
    if token:
        start = patched.find(TOKEN_PREFIX)
        end = patched.find(b":" + args.phone_make.encode() + b",", start + 1)
        if start >= 0 and end > start:
            patched = patched[:start] + token + patched[end:]

    tail_idx = patched.rfind(TAIL_CONST.encode())
    if tail_idx > 64:
        search_start = max(0, tail_idx - 128)
        window = bytes(patched[search_start:tail_idx])
        old_cipher = last_hex_run(window, 64)
        if old_cipher:
            rel = window.rfind(old_cipher)
            abs_idx = search_start + rel
            patched[abs_idx : abs_idx + 64] = cipher_hex.encode()

    if len(t3_hex) == 32 and len(patched) >= 32:
        patched[-32:] = t3_hex.encode()

    return bytes(patched) if patched else fallback


def build_xe1(args: argparse.Namespace) -> dict[str, Any]:
    request_body = args.body.encode()
    body_hash = body_md5(request_body)
    cipher_hex, t3_hex = v2_cipher(args.nonce)
    fallback = synthetic_real_msg(args, cipher_hex, t3_hex, body_hash)
    mode = "synthetic"
    real_msg = fallback
    if args.template:
        raw = Path(args.template).read_bytes()
        real_msg = patch_template(raw, fallback, args, cipher_hex, t3_hex)
        mode = f"template:{args.template}"
    sha_hex = hmac_sha256(args.display_encoder_key.encode(), real_msg).hex()
    xe1 = f"{sha_hex}:{cipher_hex}:D:{args.ts}"
    return {
        "mode": mode,
        "x_e1": xe1,
        "cipher_hex": cipher_hex,
        "t3_first16_hex": t3_hex,
        "sha_hex": sha_hex,
        "body_md5": body_hash,
        "real_msg_len": len(real_msg),
        "cipher_self_test_ok": cipher_hex == EXPECTED_CIPHER if args.nonce == DEFAULT_NONCE else None,
        "expected_full_xe1_ok": xe1 == args.expected_xe1 if args.expected_xe1 else None,
    }


def default_headers(args: argparse.Namespace, xe1: str) -> dict[str, str]:
    token = clean_token(args.token)
    headers = {
        "Authorization": f"Bearer {token}",
        "X-AppVersion": args.app_version,
        "X-Help-Version": args.app_version,
        "X-DeviceOS": args.device_os,
        "X-User-Type": "customer",
        "User-Agent": f"GoPay/{args.app_version} ({args.app_id}; build:{args.app_build}; {args.device_os})",
        "X-AppId": args.app_id,
        "Gojek-Timezone": "Asia/Jakarta",
        "Gojek-Country-Code": "ID",
        "Country-Code": "ID",
        "Gojek-Service-Area": "1",
        "X-AppType": "GOPAY",
        "X-User-Locale": "en_ID",
        "X-UniqueId": args.unique_id,
        "X-PhoneMake": args.phone_make,
        "X-PhoneModel": args.phone_model,
        "X-Location": args.location,
        "X-Location-Accuracy": args.location_accuracy,
        "X-M1": args.x_m1,
        "X-E1": xe1,
        "X-E2": args.x_e2,
        "Accept": "application/json",
        "Content-Type": "application/json",
        "Accept-Encoding": "gzip",
    }
    if args.support_request_id:
        headers["Support-Request-Id"] = args.support_request_id
    return headers


def mask(value: str, keep: int = 8) -> str:
    if len(value) <= keep * 2:
        return "***"
    return value[:keep] + "..." + value[-keep:]


def live_probe(args: argparse.Namespace, xe1: str) -> dict[str, Any]:
    if not clean_token(args.token):
        raise SystemExit("--send requires --token or GOPAY_SSO_TOKEN/GOPAY_TOKEN")
    body = args.body.encode()
    headers = default_headers(args, xe1)
    req = urllib.request.Request(args.url, data=body if args.method.upper() != "GET" else None, method=args.method.upper())
    for key, value in headers.items():
        req.add_header(key, value)
    started = time.time()
    try:
        with urllib.request.urlopen(req, timeout=args.timeout) as resp:  # noqa: S310 - explicit probe tool
            raw = resp.read(args.max_response_bytes)
            status = resp.status
            resp_headers = dict(resp.headers.items())
    except urllib.error.HTTPError as exc:
        raw = exc.read(args.max_response_bytes)
        status = exc.code
        resp_headers = dict(exc.headers.items())
    elapsed_ms = int((time.time() - started) * 1000)
    if resp_headers.get("Content-Encoding", "").lower() == "gzip":
        raw = gzip.decompress(raw)
    text = raw.decode("utf-8", errors="replace")
    return {
        "status": status,
        "elapsed_ms": elapsed_ms,
        "ok": status in args.expect_status,
        "expect_status": sorted(args.expect_status),
        "response_sample": text[: args.max_response_chars],
    }


def parse_statuses(value: str) -> set[int]:
    statuses: set[int] = set()
    for part in value.split(","):
        part = part.strip()
        if not part:
            continue
        statuses.add(int(part))
    return statuses


def add_args() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Probe GoPay X-E1 v2 signer")
    parser.add_argument("--self-test", action="store_true", help="only verify PR28 cipher test vector")
    parser.add_argument("--send", action="store_true", help="send the signed request after offline checks")
    parser.add_argument("--json", action="store_true", help="print machine-readable JSON")
    parser.add_argument("--show-secrets", action="store_true", help="print unmasked token/header material")
    parser.add_argument("--token", default=os.getenv("GOPAY_SSO_TOKEN") or os.getenv("GOPAY_TOKEN") or "")
    parser.add_argument("--url", default=os.getenv("GOPAY_PROBE_URL", DEFAULT_URL))
    parser.add_argument("--method", default=os.getenv("GOPAY_PROBE_METHOD", DEFAULT_METHOD))
    parser.add_argument("--body", default=os.getenv("GOPAY_PROBE_BODY", DEFAULT_BODY))
    parser.add_argument("--template", default=os.getenv("GOPAY_SIGNED_MSG_TEMPLATE", ""))
    parser.add_argument("--display-encoder-key", default=os.getenv("GOPAY_DISPLAY_ENCODER_KEY", DEFAULT_DISPLAY_ENCODER_KEY))
    parser.add_argument("--x-e2", default=os.getenv("GOPAY_X_E2", DEFAULT_X_E2))
    parser.add_argument("--nonce", default=os.getenv("GOPAY_V2_NONCE") or "".join(secrets.choice(string.ascii_letters + string.digits) for _ in range(32)))
    parser.add_argument("--ts", default=os.getenv("GOPAY_V2_TS") or str(int(time.time() * 1000)))
    parser.add_argument("--expected-xe1", default=os.getenv("GOPAY_EXPECTED_X_E1", ""))
    parser.add_argument("--x-m1", default=os.getenv("GOPAY_X_M1", DEFAULT_XM1))
    parser.add_argument("--unique-id", default=os.getenv("GOPAY_UNIQUE_ID", DEFAULT_UNIQUE_ID))
    parser.add_argument("--phone-make", default=os.getenv("GOPAY_PHONE_MAKE", DEFAULT_PHONE_MAKE))
    parser.add_argument("--phone-model", default=os.getenv("GOPAY_PHONE_MODEL", DEFAULT_PHONE_MODEL))
    parser.add_argument("--device-os", default=os.getenv("GOPAY_DEVICE_OS", DEFAULT_DEVICE_OS))
    parser.add_argument("--app-version", default=os.getenv("GOPAY_APP_VERSION", DEFAULT_APP_VERSION))
    parser.add_argument("--app-build", default=os.getenv("GOPAY_APP_BUILD", DEFAULT_APP_BUILD))
    parser.add_argument("--app-id", default=os.getenv("GOPAY_APP_ID", DEFAULT_APP_ID))
    parser.add_argument("--location", default=os.getenv("GOPAY_LOCATION", DEFAULT_LOCATION))
    parser.add_argument("--location-accuracy", default=os.getenv("GOPAY_LOCATION_ACCURACY", DEFAULT_LOCATION_ACCURACY))
    parser.add_argument("--support-request-id", default=os.getenv("GOPAY_SUPPORT_REQUEST_ID", ""))
    parser.add_argument("--expect-status", type=parse_statuses, default=parse_statuses(os.getenv("GOPAY_EXPECT_STATUS", "200,201,204")))
    parser.add_argument("--timeout", type=float, default=float(os.getenv("GOPAY_PROBE_TIMEOUT", "20")))
    parser.add_argument("--max-response-bytes", type=int, default=64 * 1024)
    parser.add_argument("--max-response-chars", type=int, default=2000)
    return parser


def main() -> int:
    parser = add_args()
    args = parser.parse_args()
    if args.self_test:
        args.nonce = DEFAULT_NONCE
        args.ts = DEFAULT_TS
        args.expected_xe1 = args.expected_xe1 or ""
    result = build_xe1(args)
    if args.self_test and result["cipher_hex"] != EXPECTED_CIPHER:
        print("cipher self-test failed", file=sys.stderr)
        print(json.dumps(result, ensure_ascii=False, indent=2), file=sys.stderr)
        return 1
    if args.expected_xe1 and result["expected_full_xe1_ok"] is False:
        print("full X-E1 mismatch", file=sys.stderr)
        print(json.dumps(result, ensure_ascii=False, indent=2), file=sys.stderr)
        return 2
    if args.send:
        result["live_probe"] = live_probe(args, result["x_e1"])
        if not result["live_probe"]["ok"]:
            result["live_probe_exit"] = "unexpected_status"
    if args.json:
        safe = dict(result)
        if not args.show_secrets:
            safe["x_e1"] = mask(safe["x_e1"], 12)
        print(json.dumps(safe, ensure_ascii=False, indent=2))
    else:
        print(f"cipher_self_test_ok={result['cipher_hex'] == EXPECTED_CIPHER if args.self_test else result['cipher_self_test_ok']}")
        print(f"mode={result['mode']} real_msg_len={result['real_msg_len']} body_md5={result['body_md5']}")
        print(f"cipher={result['cipher_hex']}")
        print(f"t3_first16={result['t3_first16_hex']}")
        print(f"x_e1={result['x_e1'] if args.show_secrets else mask(result['x_e1'], 12)}")
        if args.expected_xe1:
            print(f"expected_full_xe1_ok={result['expected_full_xe1_ok']}")
        if args.send:
            probe = result["live_probe"]
            print(f"live_status={probe['status']} ok={probe['ok']} elapsed_ms={probe['elapsed_ms']}")
            print(probe["response_sample"])
        elif clean_token(args.token):
            print("dry_run=true use --send to issue the request")
    if args.send and not result.get("live_probe", {}).get("ok", True):
        return 3
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
