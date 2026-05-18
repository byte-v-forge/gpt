#!/usr/bin/env python3
"""GoPay tokenization payment flow for ChatGPT Plus subscriptions.

Replays Stripe → Midtrans → GoPay's tokenization linking + charge in pure
HTTP. No browser needed. GoPay OTP is delivered by the caller through an
injected callback; the service API uses segmented start/complete calls instead
of waiting for OTP inside this module.

Flow (15 steps):

    1.  POST chatgpt.com/backend-api/payments/checkout
            body: {entry_point, plan_name, billing_details:{country:ID,currency:IDR}, ...}
            ← cs_live_xxx
    2.  POST api.stripe.com/v1/payment_methods (type=gopay)         ← pm_xxx
    3.  POST api.stripe.com/v1/payment_pages/{cs}/confirm           ← status:open
    4.  POST chatgpt.com/backend-api/payments/checkout/approve      ← approved
    5.  GET  pm-redirects.stripe.com/authorize/{nonce}              → 302 → midtrans
    6.  GET  app.midtrans.com/snap/v1/transactions/{snap_token}     ← merchant info
    7.  POST app.midtrans.com/snap/v3/accounts/{snap_token}/linking
            body: {type:gopay, country_code, phone_number}
            (406 first attempt if account already linked, retry → 201)  ← reference_id
    8.  POST gwa.gopayapi.com/v1/linking/validate-reference         ← display info
    9.  POST gwa.gopayapi.com/v1/linking/user-consent               ← OTP triggered
    10. POST gwa.gopayapi.com/v1/linking/validate-otp               ← challenge_id, client_id
    11. POST customer.gopayapi.com/api/v1/users/pin/tokens/nb       ← pin_token (JWT)
    12. POST gwa.gopayapi.com/v1/linking/validate-pin               ← linking complete
    13. POST app.midtrans.com/snap/v2/transactions/{snap}/charge    ← charge_ref (A12...)
    14. tokenization=true: complete GoPay web challenge/PIN/process.
        tokenization=false: user opens Midtrans GoPay deeplink/QR and pays externally.
    15. Poll Midtrans status, then verify ChatGPT Plus.
"""

from __future__ import annotations

import argparse
import base64
import http.server
import json
import os
import re
import socketserver
import sys
import tempfile
import threading
import time
import uuid
from pathlib import Path
from typing import Any, Callable, Optional
from urllib.parse import parse_qs, quote, urlparse

import requests

# Cloudflare 拦 plain requests 的 TLS 指纹（403 + HTML challenge），用 curl_cffi
# 模拟真 Chrome 指纹。
try:
    from curl_cffi.requests import Session as _CurlCffiSession  # type: ignore
except ImportError:
    _CurlCffiSession = None  # type: ignore


def _new_session(impersonate: str = "chrome136") -> Any:
    """Build session with chrome TLS fingerprint when available."""
    if _CurlCffiSession is not None:
        return _CurlCffiSession(impersonate=impersonate)
    return requests.Session()


def _clean_proxy(value: Any) -> str:
    return str(value or "").strip()


def _set_session_proxy(session: Any, proxy: Optional[str]) -> None:
    try:
        value = _clean_proxy(proxy)
        session.proxies = {"http": value, "https": value} if value else {}
    except Exception:
        pass


def _cfg_proxy(cfg: dict, *keys: str) -> str:
    for key in keys:
        value: Any = cfg
        for part in key.split("."):
            if not isinstance(value, dict):
                value = ""
                break
            value = value.get(part)
        cleaned = _clean_proxy(value)
        if cleaned:
            return cleaned
    return ""


_SESSION_PAID_BOOL_KEYS = {
    "haspaid",
    "haspaidsubscription",
    "hasactivepaidsubscription",
    "hasactivesubscription",
    "hassubscription",
    "ispaid",
    "ispaidaccount",
    "ispaidsubscriptionactive",
    "ispaiduser",
    "isplus",
    "isplusaccount",
    "isplususer",
    "plusactive",
    "subscribed",
    "issubscribed",
    "subscriber",
}
_SESSION_PLAN_KEYS = {
    "accountplan",
    "accountplantype",
    "billingplan",
    "license",
    "plantype",
    "plan",
    "product",
    "productid",
    "sku",
    "subscription",
    "subscriptionplan",
    "subscriptiontier",
    "tier",
}
_SESSION_GROUP_KEYS = {"entitlement", "entitlements", "group", "groups", "roles"}
_SESSION_ACTIVE_STATUSES = {"active", "paid", "subscribed", "trialing"}
_SESSION_PAID_PLAN_RE = re.compile(r"(^|[_:/\-\s])(chatgpt[_:/\-\s]*)?(plus|pro|team|business|enterprise)([_:/\-\s]|$)")
_SESSION_FREE_PLAN_RE = re.compile(r"(^|[_:/\-\s])(free|none|anonymous|unauthenticated)([_:/\-\s]|$)")
_SESSION_COOKIE_NAME = "__Secure-next-auth.session-token"
_SESSION_COOKIE_FALLBACK_NAME = "next-auth.session-token"
_SESSION_COOKIE_CHUNK_SIZE = 4096 - 163


def _session_key(value: Any) -> str:
    return re.sub(r"[^a-z0-9]+", "", str(value or "").lower())


def _session_string_plan(value: Any) -> str:
    text = str(value or "").strip().lower()
    if not text:
        return ""
    if _SESSION_PAID_PLAN_RE.search(text):
        match = re.search(r"(plus|pro|team|business|enterprise)", text)
        return match.group(1) if match else text[:80]
    if _SESSION_FREE_PLAN_RE.search(text):
        return "free"
    return ""


def _session_cookie_name(name: str) -> bool:
    clean = str(name or "").strip()
    for base in (_SESSION_COOKIE_NAME, _SESSION_COOKIE_FALLBACK_NAME):
        if clean == base:
            return True
        if clean.startswith(base + ".") and clean[len(base) + 1:].isdigit():
            return True
    return False


def _cookie_name(part: str) -> str:
    return str(part or "").split("=", 1)[0].strip()


def _ordered_session_cookie_parts(parts: list[str]) -> list[str]:
    def sort_key(part: str) -> tuple[int, int, str]:
        name = _cookie_name(part)
        for base_order, base in enumerate((_SESSION_COOKIE_NAME, _SESSION_COOKIE_FALLBACK_NAME)):
            if name == base:
                return (base_order, -1, name)
            prefix = base + "."
            if name.startswith(prefix):
                suffix = name[len(prefix):]
                if suffix.isdigit():
                    return (base_order, int(suffix), name)
        return (99, 0, name)

    return sorted(parts, key=sort_key)


def _session_token_from_json(value: str) -> str:
    text = str(value or "").strip()
    if not text.startswith("{"):
        return ""
    try:
        payload = json.loads(text)
    except Exception:
        return ""
    if not isinstance(payload, dict):
        return ""
    for key in ("sessionToken", "session_token"):
        token = str(payload.get(key) or "").strip()
        if token:
            return token
    return ""


def _session_cookie_parts(value: str) -> list[str]:
    """Build only the NextAuth session-cookie pieces from a token or Cookie header."""
    raw = str(value or "").strip()
    if not raw:
        return []
    if raw.lower().startswith("cookie:"):
        raw = raw.split(":", 1)[1].strip()

    json_token = _session_token_from_json(raw)
    if json_token:
        raw = json_token

    if "=" in raw:
        found: list[str] = []
        for chunk in raw.split(";"):
            part = chunk.strip().strip("'\"")
            if "=" not in part:
                continue
            name, cookie_value = part.split("=", 1)
            name = name.strip()
            cookie_value = cookie_value.strip().strip("'\"")
            if _session_cookie_name(name) and cookie_value:
                found.append(f"{name}={cookie_value}")
        if found:
            return _ordered_session_cookie_parts(found)

    token = raw.strip().strip("'\"")
    if not token:
        return []
    if len(token) <= _SESSION_COOKIE_CHUNK_SIZE:
        return [f"{_SESSION_COOKIE_NAME}={token}"]
    return [
        f"{_SESSION_COOKIE_NAME}.{index}={token[offset:offset + _SESSION_COOKIE_CHUNK_SIZE]}"
        for index, offset in enumerate(range(0, len(token), _SESSION_COOKIE_CHUNK_SIZE))
    ]


def _normalize_tier(value: Any) -> str:
    tier = str(value or "").strip().lower()
    if tier in {"plus", "pro", "team", "business", "enterprise", "free"}:
        return tier
    return _session_string_plan(tier)


def _decode_jwt_payload(token: Any) -> dict:
    parts = str(token or "").split(".")
    if len(parts) < 2:
        return {}
    payload = parts[1]
    padding = "=" * (-len(payload) % 4)
    try:
        decoded = base64.urlsafe_b64decode((payload + padding).encode("ascii"))
        data = json.loads(decoded.decode("utf-8"))
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def _tier_result(tier: str, source: str) -> dict:
    normalized = _normalize_tier(tier)
    if not normalized:
        return {}
    return {
        "plus_active": normalized != "free",
        "plan_type": normalized,
        "tier": normalized,
        "source": source,
    }


def _access_token_auth_claims(access_token: Any) -> dict:
    token_payload = _decode_jwt_payload(access_token)
    auth_claims = token_payload.get("https://api.openai.com/auth")
    return auth_claims if isinstance(auth_claims, dict) else {}


def _access_token_tier(access_token: Any, source_prefix: str = "accessToken.auth") -> dict:
    auth_claims = _access_token_auth_claims(access_token)
    for key in ("chatgpt_plan_type", "chatgpt_planType", "plan_type", "planType"):
        result = _tier_result(auth_claims.get(key), f"{source_prefix}.{key}")
        if result:
            return result
    return {}


def _access_token_account_id(access_token: Any) -> str:
    auth_claims = _access_token_auth_claims(access_token)
    for key in ("chatgpt_account_id", "account_id"):
        value = str(auth_claims.get(key) or "").strip()
        if value:
            return value
    return ""


def _explicit_session_tier(payload: Any) -> dict:
    if not isinstance(payload, dict):
        return {}

    token_result = _access_token_tier(payload.get("accessToken"))
    if token_result:
        return token_result

    account = payload.get("account")
    if isinstance(account, dict):
        for key in ("planType", "plan_type", "accountPlan", "account_plan", "tier", "plan"):
            result = _tier_result(account.get(key), f"account.{key}")
            if result:
                return result

    return {}


def _detect_plus_active_from_session_payload(payload: Any) -> dict:
    """Best-effort parser for ChatGPT /api/auth/session subscription markers."""
    explicit = _explicit_session_tier(payload)
    if explicit:
        return {
            "checked": True,
            "plus_active": bool(explicit.get("plus_active")),
            "plan_type": str(explicit.get("plan_type") or ""),
            "tier": str(explicit.get("tier") or ""),
            "source": "auth_session:" + str(explicit.get("source") or "$"),
            "error_message": "",
        }

    best_free: dict[str, str] = {}

    def walk(value: Any, path: tuple[str, ...] = ()) -> Optional[dict]:
        nonlocal best_free
        key = _session_key(path[-1]) if path else ""
        source = ".".join(path) or "$"

        if isinstance(value, bool):
            if value and key in _SESSION_PAID_BOOL_KEYS:
                return {"plus_active": True, "plan_type": "paid", "source": source}
            return None

        if isinstance(value, str):
            plan = _session_string_plan(value)
            path_keys = {_session_key(part) for part in path}
            plan_key = bool(path_keys & (_SESSION_PLAN_KEYS | _SESSION_GROUP_KEYS))
            contextual_key = any(
                part in path_key
                for path_key in path_keys
                for part in ("plan", "subscription", "tier", "product", "sku", "license", "entitlement", "group", "role")
            )
            if plan and (plan_key or contextual_key):
                if plan == "free":
                    if not best_free:
                        best_free = {"plan_type": plan, "source": source}
                    return None
                return {"plus_active": True, "plan_type": plan, "source": source}
            return None

        if isinstance(value, dict):
            status = ""
            for status_key in ("status", "subscription_status", "subscriptionStatus", "state"):
                raw_status = value.get(status_key)
                if isinstance(raw_status, str):
                    status = raw_status.strip().lower()
                    break
            local_plan = ""
            local_plan_source = ""
            for plan_key in ("plan", "plan_type", "planType", "account_plan", "accountPlan", "tier", "sku", "product"):
                plan = _session_string_plan(value.get(plan_key))
                if plan:
                    local_plan = plan
                    local_plan_source = ".".join(path + (plan_key,))
                    break
            if local_plan and local_plan != "free" and (not status or status in _SESSION_ACTIVE_STATUSES):
                return {"plus_active": True, "plan_type": local_plan, "source": local_plan_source or source}
            if local_plan == "free" and not best_free:
                best_free = {"plan_type": "free", "source": local_plan_source or source}

            for child_key, child_value in value.items():
                hit = walk(child_value, path + (str(child_key),))
                if hit:
                    return hit
            return None

        if isinstance(value, list):
            for index, child in enumerate(value):
                hit = walk(child, path + (str(index),))
                if hit:
                    return hit
            return None

        return None

    hit = walk(payload)
    if hit:
        return {
            "checked": True,
            "plus_active": True,
            "plan_type": _normalize_tier(hit.get("plan_type")) or "paid",
            "tier": _normalize_tier(hit.get("plan_type")) or "paid",
            "source": "auth_session:" + str(hit.get("source") or "$"),
            "error_message": "",
        }
    checked = isinstance(payload, dict) and ("user" in payload or "accessToken" in payload)
    tier = best_free.get("plan_type", "")
    if checked and not tier:
        tier = "free"
    return {
        "checked": checked,
        "plus_active": False,
        "plan_type": tier,
        "tier": tier,
        "source": "auth_session:" + best_free.get("source", "no_paid_marker"),
        "error_message": "",
    }


def probe_tier_access_token(
    access_token: str,
    *,
    account_id: str = "",
    proxy: Optional[str] = None,
    log: Callable[[str], None] = print,
) -> dict:
    """Probe tier from ChatGPT backend bearer endpoints; fallback is handled by caller."""
    token = str(access_token or "").strip()
    if not token:
        return {
            "checked": False,
            "plus_active": False,
            "plan_type": "",
            "tier": "",
            "source": "wham_usage",
            "error_message": "access_token is required",
        }

    session = _new_session()
    try:
        _set_session_proxy(session, proxy)
        session.headers.update({
            "Authorization": f"Bearer {token}",
            "Accept": "application/json",
            "User-Agent": "codex-cli",
        })
        resolved_account_id = str(account_id or "").strip() or _access_token_account_id(token)
        if resolved_account_id:
            session.headers["ChatGPT-Account-Id"] = resolved_account_id
        resp = _request_with_retries(
            session,
            "get",
            "https://chatgpt.com/backend-api/wham/usage",
            log=log,
            timeout=DEFAULT_TIMEOUT,
        )
        if resp.status_code != 200:
            return {
                "checked": False,
                "plus_active": False,
                "plan_type": "",
                "tier": "",
                "source": "wham_usage",
                "error_message": f"wham usage returned status {resp.status_code}",
            }
        payload = resp.json() or {}
        result = _tier_result(payload.get("plan_type") or payload.get("planType"), "wham_usage.plan_type")
        if not result:
            return {
                "checked": True,
                "plus_active": False,
                "plan_type": "",
                "tier": "",
                "source": "wham_usage",
                "error_message": "wham usage returned no plan_type",
            }
        return {
            "checked": True,
            "plus_active": bool(result.get("plus_active")),
            "plan_type": str(result.get("plan_type") or ""),
            "tier": str(result.get("tier") or ""),
            "source": str(result.get("source") or "wham_usage.plan_type"),
            "error_message": "",
        }
    except Exception as exc:
        return {
            "checked": False,
            "plus_active": False,
            "plan_type": "",
            "tier": "",
            "source": "wham_usage",
            "error_message": f"wham usage probe failed: {str(exc)[:500]}",
        }
    finally:
        close = getattr(session, "close", None)
        if callable(close):
            try:
                close()
            except Exception:
                pass


def probe_plus_active_session_token(
    session_token: str,
    *,
    proxy: Optional[str] = None,
    log: Callable[[str], None] = print,
) -> dict:
    """Probe ChatGPT Plus state directly from /api/auth/session using a session token."""
    token = str(session_token or "").strip()
    if not token:
        return {
            "checked": False,
            "plus_active": False,
            "plan_type": "",
            "source": "auth_session",
            "error_message": "session_token is required",
        }

    session = _new_session()
    try:
        _set_session_proxy(session, proxy)
        session.headers.update({
            "User-Agent": (
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
            ),
            "Accept": "application/json",
            "Accept-Language": "en-US,en;q=0.9",
            "Referer": "https://chatgpt.com/",
            "Cookie": "; ".join(_session_cookie_parts(token)),
        })
        resp = _request_with_retries(
            session,
            "get",
            "https://chatgpt.com/api/auth/session",
            log=log,
            timeout=DEFAULT_TIMEOUT,
        )
        if resp.status_code != 200:
            return {
                "checked": False,
                "plus_active": False,
                "plan_type": "",
                "source": "auth_session",
                "error_message": f"auth session returned status {resp.status_code}",
            }
        payload = resp.json() or {}
        access_token = str(payload.get("accessToken") or "").strip() if isinstance(payload, dict) else ""
        if access_token:
            wham_result = probe_tier_access_token(
                access_token,
                account_id=_access_token_account_id(access_token),
                proxy=proxy,
                log=log,
            )
            if wham_result.get("checked") and (wham_result.get("tier") or wham_result.get("plan_type")):
                return wham_result

            token_result = _access_token_tier(access_token)
            if token_result:
                return {
                    "checked": True,
                    "plus_active": bool(token_result.get("plus_active")),
                    "plan_type": str(token_result.get("plan_type") or ""),
                    "tier": str(token_result.get("tier") or ""),
                    "source": str(token_result.get("source") or "accessToken.auth"),
                    "error_message": "",
                }

        result = _detect_plus_active_from_session_payload(payload)
        if not result.get("checked"):
            result["error_message"] = "auth session returned no authenticated user"
        return result
    except Exception as exc:
        return {
            "checked": False,
            "plus_active": False,
            "plan_type": "",
            "source": "auth_session",
            "error_message": f"auth session probe failed: {str(exc)[:500]}",
        }
    finally:
        close = getattr(session, "close", None)
        if callable(close):
            try:
                close()
            except Exception:
                pass


# ──────────────────────────── constants ───────────────────────────

# OpenAI's Midtrans merchant client id (public, embedded in JS).
# Override via gopay config block if rotated.
DEFAULT_MIDTRANS_CLIENT_ID = "Mid-client-3TX8nUa-f_RgNrky"

# OpenAI's Stripe live publishable key (public, embedded in checkout page JS).
# Override via cfg["stripe"]["publishable_key"] if it ever changes.
DEFAULT_STRIPE_PK = (
    "pk_live_51HOrSwC6h1nxGoI3lTAgRjYVrz4dU3fVOabyCcKR3pbEJguCVAlqCxdxCUvoRh1XWwRac"
    "ViovU3kLKvpkjh7IqkW00iXQsjo3n"
)
DEFAULT_STRIPE_HCAPTCHA_ASSET_VERSION = "v32.5"
HCAPTCHA_SITE_KEY_FALLBACK = "c7faac4c-1cd7-4b1b-b2d4-42ba98d09c7a"

GOPAY_PIN_CLIENT_ID_LINK = "51b5f09a-3813-11ee-be56-0242ac120002-MGUPA"
GOPAY_PIN_CLIENT_ID_CHARGE = "47180a8e-f56e-11ed-a05b-0242ac120003-GWC"

DEFAULT_TIMEOUT = 30
LINK_RETRY_LIMIT = 2  # 406 "account already linked" retry
LINK_RETRY_SLEEP_S = 12.0  # Midtrans 需要冷却 ~10s 才会让 406 → 201（实测）
# 429 "There's a technical error" 风控触发条件：带 Authorization 的 SDK 路径
# 在某些 IP / 高频场景必现。剥掉 Authorization 头同 endpoint 重发即返回 201
# + activation_link_url（实测 + 反向工程参考实现确认）。
LINK_BYPASS_BODY_HINTS = (
    "technical error",
    "too many",
    "rate limit",
    "rate_limit",
)
MIDTRANS_STATUS_POLL_LIMIT = 12
RETRYABLE_TRANSPORT_HINTS = (
    "tls connect error",
    "connection reset",
    "connection aborted",
    "timed out",
    "timeout",
    "temporarily unavailable",
    "network is unreachable",
    "proxy",
    "eof",
)


# ──────────────────────────── exceptions ──────────────────────────


class GoPayError(RuntimeError):
    pass


class OTPCancelled(GoPayError):
    pass


class GoPayOTPRejected(GoPayError):
    pass


class GoPayPINRejected(GoPayError):
    pass


def _is_retryable_transport_error(exc: Exception) -> bool:
    text = str(exc).lower()
    return any(hint in text for hint in RETRYABLE_TRANSPORT_HINTS)


def _json_excerpt(value: Any, limit: int = 600) -> str:
    try:
        text = json.dumps(_redact_for_log(value), ensure_ascii=False, separators=(",", ":"))
    except Exception:
        text = _redact_text_for_log(value)
    return text[:limit]


def _response_excerpt(response: Any, limit: int = 600) -> str:
    text = str(getattr(response, "text", "") or "").strip()
    try:
        payload = response.json()
        if payload not in ({}, [], None, "") or not text:
            return _json_excerpt(payload, limit=limit)
    except Exception:
        pass
    if not text:
        text = "<empty response>"
    try:
        return _json_excerpt(json.loads(text), limit=limit)
    except Exception:
        return _redact_text_for_log(text)[:limit]


def _normalize_otp_channel(value: str) -> str:
    value = str(value or "").strip().lower()
    if value in {"sms", "otp_sms"}:
        return "sms"
    if value in {"wa", "whatsapp", "otp_wa"}:
        return "wa"
    return ""


def _find_bool_field(value: Any, field_names: set[str]) -> Optional[bool]:
    if isinstance(value, dict):
        for key, item in value.items():
            if str(key).lower() in field_names and isinstance(item, bool):
                return item
            found = _find_bool_field(item, field_names)
            if found is not None:
                return found
    elif isinstance(value, list):
        for item in value:
            found = _find_bool_field(item, field_names)
            if found is not None:
                return found
    return None


def _linking_consent_requires_otp(value: Any) -> bool:
    found = _find_bool_field(
        value,
        {"otp_required", "is_otp_required", "requires_otp", "need_otp", "needs_otp"},
    )
    if found is not None:
        return found
    return True


def _redact_for_log(value: Any, key: str = "") -> Any:
    if isinstance(value, dict):
        return {item_key: _redact_for_log(item_value, str(item_key)) for item_key, item_value in value.items()}
    if isinstance(value, list):
        return [_redact_for_log(item, key) for item in value]
    if _is_url_log_key(key):
        return "<redacted-url>" if value not in (None, "") else value
    if _is_sensitive_log_key(key):
        return "<redacted>" if value not in (None, "") else value
    if isinstance(value, str):
        return _redact_text_for_log(value)
    return value


def _is_url_log_key(key: str) -> bool:
    lower = key.lower()
    return any(part in lower for part in ("url", "uri", "deeplink", "redirect", "qr_code", "qrcode"))


def _is_sensitive_log_key(key: str) -> bool:
    lower = key.lower()
    parts = [part for part in re.split(r"[^a-z0-9]+", lower) if part]
    if lower in {"authorization", "cookie", "pin", "otp", "password", "passwd", "secret"}:
        return True
    if any(part in {"authorization", "cookie", "pin", "otp", "password", "passwd", "secret"} for part in parts):
        return True
    if "token" in parts or lower.endswith("_token"):
        return True
    if lower in {"challenge_id", "client_id", "snap_token", "session"}:
        return True
    return False


def _redact_text_for_log(value: Any) -> str:
    text = str(value)
    text = re.sub(r"(?i)\b(?:https?|gopay)://[^\s\"'<>]+", "<redacted-url>", text)
    text = re.sub(r"(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{12,}", "Bearer <redacted>", text)
    text = re.sub(
        r"(?i)\b(access_token|refresh_token|session_token|csrf_token|token|pin|otp|password|secret|cookie)"
        r"\s*[:=]\s*['\"]?[^'\"\s,}]{6,}",
        lambda match: f"{match.group(1)}=<redacted>",
        text,
    )
    text = re.sub(r"\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b", "<redacted-token>", text)
    text = re.sub(r"\b[A-Za-z0-9_-]{48,}\b", "<redacted-token>", text)
    return text


def _iter_json_strings(value: Any, path: str = ""):
    if isinstance(value, dict):
        for key, item in value.items():
            item_path = f"{path}.{key}" if path else str(key)
            yield from _iter_json_strings(item, item_path)
    elif isinstance(value, list):
        for index, item in enumerate(value):
            yield from _iter_json_strings(item, f"{path}[{index}]")
    elif isinstance(value, str):
        yield path, value


def _extract_reference_from_text(text: str) -> str:
    text = (text or "").strip()
    if not text:
        return ""

    parsed = urlparse(text)
    query = parse_qs(parsed.query)
    for key in ("reference", "reference_id", "referenceId", "tref"):
        value = next((item.strip() for item in query.get(key, []) if item.strip()), "")
        if value:
            return value

    match = re.search(r"(?:[?&#]|^)(?:reference|reference_id|referenceId)=([A-Za-z0-9-]+)", text)
    return match.group(1) if match else ""


def _extract_checkout_session_id(data: dict[str, Any]) -> str:
    for key in ("checkout_session_id", "session_id", "id"):
        value = str(data.get(key) or "").strip()
        if value.startswith("cs_"):
            return value
    for key in ("url", "stripe_hosted_url", "checkout_url"):
        text = str(data.get(key) or "")
        match = re.search(r"(cs_(?:live|test)_[A-Za-z0-9]+)", text)
        if match:
            return match.group(1)
    return ""


def _checkout_url_from_response(data: dict[str, Any], cs_id: str) -> str:
    for key in ("url", "stripe_hosted_url", "checkout_url"):
        value = str(data.get(key) or "").strip()
        if value:
            return value
    return f"https://checkout.stripe.com/c/pay/{cs_id}" if cs_id else ""


def _extract_midtrans_charge_reference(data: Any) -> str:
    for _, text in _iter_json_strings(data):
        reference = _extract_reference_from_text(text)
        if reference:
            return reference

    for path, text in _iter_json_strings(data):
        if "reference" in path.lower() and re.fullmatch(r"[A-Za-z0-9-]{6,}", text.strip()):
            return text.strip()
    return ""


def _extract_midtrans_url(data: Any, *names: str) -> str:
    if not isinstance(data, dict):
        return ""
    wanted = {name.lower() for name in names}
    for name in names:
        value = str(data.get(name) or "").strip()
        if value:
            return value
    for action in data.get("actions") or []:
        if not isinstance(action, dict):
            continue
        name = str(action.get("name") or "").strip().lower()
        if name in wanted:
            value = str(action.get("url") or "").strip()
            if value:
                return value
    return ""


def _midtrans_charge_urls(data: Any) -> dict[str, str]:
    return {
        "deeplink_url": _extract_midtrans_url(data, "deeplink_url", "deeplink"),
        "qr_code_url": _extract_midtrans_url(data, "qr_code_url", "qr_code", "qrcode"),
        "finish_redirect_url": _extract_midtrans_url(data, "finish_redirect_url"),
        "finish_200_redirect_url": _extract_midtrans_url(data, "finish_200_redirect_url"),
    }


def _midtrans_charge_denial_message(data: dict[str, Any]) -> str:
    status = str(data.get("transaction_status") or "").strip()
    fraud_status = str(data.get("fraud_status") or "").strip()
    status_code = str(data.get("status_code") or "").strip()
    if status not in {"deny", "cancel", "expire", "failure"} and fraud_status != "deny":
        return ""

    parts = ["midtrans charge denied"]
    if status_code:
        parts.append(f"status_code={status_code}")
    if status:
        parts.append(f"transaction_status={status}")
    if fraud_status:
        parts.append(f"fraud_status={fraud_status}")
    for key in ("status_message", "order_id", "transaction_id", "gross_amount", "currency"):
        value = str(data.get(key) or "").strip()
        if value:
            parts.append(f"{key}={value}")
    return " ".join(parts)


def _request_with_retries(
    session: Any,
    method: str,
    url: str,
    *,
    log: Callable[[str], None],
    attempts: int = 3,
    delay_seconds: float = 1.0,
    **kwargs: Any,
) -> Any:
    request = getattr(session, method)
    host = re.sub(r"^https?://([^/]+).*", r"\1", url)
    last_exc: Optional[Exception] = None
    for attempt in range(1, max(1, attempts) + 1):
        try:
            return request(url, **kwargs)
        except Exception as exc:
            last_exc = exc
            if attempt >= attempts or not _is_retryable_transport_error(exc):
                raise
            log(
                f"[gopay] {method.upper()} {host} transient transport error "
                f"({attempt}/{attempts}): {type(exc).__name__}: {str(exc)[:160]}"
            )
            time.sleep(delay_seconds * attempt)
    raise last_exc or GoPayError(f"{method.upper()} {host} failed")


def _build_stripe_hcaptcha_url(
    invisible: bool = True,
    frame_id: str = "",
    origin: str = "https://js.stripe.com",
) -> str:
    frame = frame_id or str(uuid.uuid4())
    page_name = "HCaptchaInvisible.html" if invisible else "HCaptcha.html"
    return (
        "https://b.stripecdn.com/stripethirdparty-srv/assets/"
        f"{DEFAULT_STRIPE_HCAPTCHA_ASSET_VERSION}/{page_name}"
        f"?id={frame}&origin={quote(origin, safe='')}"
    )


def _extract_passive_captcha_config(init_data: dict) -> dict:
    raw = json.dumps(init_data or {}, separators=(",", ":"))
    passive = init_data.get("passive_captcha") if isinstance(init_data.get("passive_captcha"), dict) else {}
    site_key = passive.get("site_key") or init_data.get("site_key") or ""
    rqdata = passive.get("rqdata")
    if rqdata is None:
        rqdata = init_data.get("rqdata", "")

    if not site_key:
        match = re.search(r'"hcaptcha_site_key"\s*:\s*"([^"]+)"', raw)
        if match:
            site_key = match.group(1)
    if not rqdata:
        match = re.search(r'"hcaptcha_rqdata"\s*:\s*"([^"]+)"', raw)
        if match:
            rqdata = match.group(1)

    return {
        "site_key": site_key or HCAPTCHA_SITE_KEY_FALLBACK,
        "rqdata": rqdata or "",
        "is_invisible": True,
        "website_url": _build_stripe_hcaptcha_url(invisible=True),
    }


def _accept_language_for_locale(locale_value: str | None) -> str:
    locale = (locale_value or "").strip().lower()
    if locale.startswith("zh"):
        return "zh-CN,zh;q=0.9,en;q=0.8"
    if locale.startswith("id"):
        return "id-ID,id;q=0.9,en;q=0.8"
    return "en-US,en;q=0.9"


def _playwright_proxy(proxy_url: str) -> Optional[dict]:
    proxy_url = (proxy_url or "").strip()
    if not proxy_url:
        return None
    try:
        parsed = urlparse(proxy_url)
        host = parsed.hostname or ""
        if not host:
            return None
        server = f"{parsed.scheme or 'http'}://{host}"
        if parsed.port:
            server += f":{parsed.port}"
        proxy = {"server": server, "bypass": "127.0.0.1,localhost"}
        if parsed.username:
            proxy["username"] = parsed.username
        if parsed.password:
            proxy["password"] = parsed.password
        return proxy
    except Exception:
        return None


def _build_stripe_hcaptcha_parent_html(
    frame_id: str,
    wrapper_url: str,
    site_key: str,
    rqdata: str,
    merchant_id: str,
    locale: str,
) -> str:
    init_payload = {
        "tag": "INITIALIZE_HCAPTCHA_INVISIBLE",
        "message": {"sitekey": site_key},
    }
    execute_payload = {
        "tag": "EXECUTE_HCAPTCHA_INVISIBLE",
        "message": {
            "sitekey": site_key,
            "rqdata": rqdata,
            "data": {
                "merchant_id": merchant_id or "",
                "locale": locale or "",
                "flow": "passive_captcha",
                "captcha_vendor": "hcaptcha",
            },
        },
    }
    signal_payloads = [
        {
            "tag": "SEND_FRAUD_SIGNALS_HCAPTCHA_INVISIBLE",
            "message": {"type": "mouse", "eventName": "mousemove", "coordinates": {"x": 168, "y": 132}},
        },
        {
            "tag": "SEND_FRAUD_SIGNALS_HCAPTCHA_INVISIBLE",
            "message": {"type": "pointer", "eventName": "pointermove", "coordinates": {"x": 214, "y": 176}},
        },
        {
            "tag": "SEND_FRAUD_SIGNALS_HCAPTCHA_INVISIBLE",
            "message": {"type": "keyboard", "eventName": "keydown"},
        },
    ]
    return f"""<!doctype html>
<html>
  <head><meta charset="utf-8"><title>Stripe hCaptcha Bridge</title></head>
  <body>
    <iframe id="stripeCaptchaFrame" src="{wrapper_url}" style="width:420px;height:720px;border:0"></iframe>
    <script>
      const frameID = {json.dumps(frame_id)};
      const initPayload = {json.dumps(init_payload, ensure_ascii=False)};
      const executePayload = {json.dumps(execute_payload, ensure_ascii=False)};
      const signalPayloads = {json.dumps(signal_payloads, ensure_ascii=False)};
      let initialized = false;
      let executed = false;

      function postToBridge(path, payload) {{
        fetch(path, {{
          method: "POST",
          headers: {{"Content-Type": "application/json"}},
          body: JSON.stringify(payload || {{}}),
          keepalive: true,
        }}).catch(() => {{}});
      }}

      function postToChild(source, origin, payload) {{
        source.postMessage({{
          type: "stripe-third-party-parent-to-child",
          frameID,
          payload,
        }}, origin);
      }}

      function initialize(source, origin) {{
        if (initialized) return;
        initialized = true;
        postToBridge("/event", {{type: "invisible_initialize"}});
        postToChild(source, origin, initPayload);
      }}

      function execute(source, origin) {{
        if (executed) return;
        executed = true;
        signalPayloads.forEach((payload, idx) => setTimeout(() => postToChild(source, origin, payload), 50 * idx));
        setTimeout(() => {{
          postToBridge("/event", {{type: "invisible_execute"}});
          postToChild(source, origin, executePayload);
        }}, 180);
      }}

      window.addEventListener("message", (event) => {{
        const data = event.data || {{}};
        if (data.type === "stripe-third-party-frame-ready" && data.frameID === frameID) {{
          postToBridge("/event", {{type: "frame_ready", origin: event.origin}});
          initialize(event.source, event.origin);
          return;
        }}
        if (data.type !== "stripe-third-party-child-to-parent" || data.frameID !== frameID) return;
        const payload = data.payload || {{}};
        postToBridge("/event", {{type: "child_payload", tag: payload.tag || ""}});
        if (payload.tag === "LOAD_HCAPTCHA_INVISIBLE") {{
          execute(event.source, event.origin);
          return;
        }}
        if (payload.tag === "RESPONSE_HCAPTCHA_INVISIBLE") {{
          const value = payload.value || {{}};
          postToBridge("/result", {{
            response: value.response || "",
            ekey: value.key || "",
            duration: value.duration || 0,
            raw: payload,
          }});
          return;
        }}
        if (payload.tag === "ERROR_HCAPTCHA_INVISIBLE") {{
          postToBridge("/error", {{error: (payload.value || {{}}).error || "unknown_error", raw: payload}});
        }}
      }});
    </script>
  </body>
</html>
"""


def _solve_passive_hcaptcha_in_browser(
    hcaptcha_config: dict,
    *,
    browser_cfg: dict,
    merchant_id: str,
    locale: str,
    log: Callable[[str], None] = print,
) -> tuple[str, str]:
    if not bool((browser_cfg or {}).get("enabled", True)):
        raise GoPayError("approve blocked and browser_challenge is disabled")
    if not bool((browser_cfg or {}).get("use_for_passive_captcha", True)):
        raise GoPayError("approve blocked and browser_challenge.use_for_passive_captcha is disabled")

    timeout_ms = int(
        (browser_cfg or {}).get("passive_timeout_ms")
        or (browser_cfg or {}).get("timeout_ms")
        or 120000
    )
    headless = bool((browser_cfg or {}).get("passive_headless", True))
    viewport = (browser_cfg or {}).get("viewport") or {"width": 1280, "height": 960}
    proxy_url = str(
        (browser_cfg or {}).get("passive_proxy_url")
        or (browser_cfg or {}).get("proxy_url")
        or ""
    ).strip()
    site_key = hcaptcha_config["site_key"]
    rqdata = hcaptcha_config.get("rqdata", "")

    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        raise GoPayError(f"approve blocked and browser_challenge requires playwright: {exc}") from exc

    with tempfile.TemporaryDirectory(prefix="stripe-hcaptcha-bridge-") as tmpdir:
        bridge_state = {"events": [], "result": None, "error": None}
        result_event = threading.Event()
        error_event = threading.Event()

        class QuietHandler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=tmpdir, **kwargs)

            def log_message(self, fmt, *args):
                return

            def _write_json(self, status: int, payload: dict):
                body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
                self.send_response(status)
                self.send_header("Content-Type", "application/json; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_POST(self):
                try:
                    length = int(self.headers.get("Content-Length", "0"))
                except ValueError:
                    length = 0
                raw_body = self.rfile.read(length) if length > 0 else b"{}"
                try:
                    payload = json.loads(raw_body.decode("utf-8") or "{}")
                except Exception:
                    payload = {}
                if self.path == "/event":
                    bridge_state["events"].append(payload)
                    self._write_json(200, {"ok": True})
                    return
                if self.path == "/result":
                    bridge_state["result"] = payload
                    result_event.set()
                    self._write_json(200, {"ok": True})
                    return
                if self.path == "/error":
                    bridge_state["error"] = payload
                    error_event.set()
                    self._write_json(200, {"ok": True})
                    return
                self._write_json(404, {"error": "not found"})

        class BridgeServer(socketserver.ThreadingMixIn, socketserver.TCPServer):
            allow_reuse_address = True
            daemon_threads = True

        httpd = BridgeServer(("127.0.0.1", 0), QuietHandler)
        port = httpd.server_address[1]
        origin = f"http://127.0.0.1:{port}"
        frame_id = str(uuid.uuid4())
        wrapper_url = _build_stripe_hcaptcha_url(invisible=True, frame_id=frame_id, origin=origin)
        html = _build_stripe_hcaptcha_parent_html(
            frame_id=frame_id,
            wrapper_url=wrapper_url,
            site_key=site_key,
            rqdata=rqdata,
            merchant_id=merchant_id,
            locale=locale,
        )
        with open(os.path.join(tmpdir, "index.html"), "w", encoding="utf-8") as f:
            f.write(html)
        server_thread = threading.Thread(target=httpd.serve_forever, daemon=True)
        server_thread.start()

        playwright_ctx = None
        browser = None
        page = None
        try:
            log(
                "[gopay] browser passive hCaptcha "
                f"site_key={site_key[:12]} rqdata={'yes' if rqdata else 'no'} headless={headless}"
            )
            playwright_ctx = sync_playwright().start()
            launch_kwargs = {
                "headless": headless,
                "args": ["--no-sandbox", "--disable-dev-shm-usage"],
            }
            chromium_executable = str(
                (browser_cfg or {}).get("chromium_executable")
                or os.environ.get("PLAYWRIGHT_CHROMIUM_EXECUTABLE")
                or ""
            ).strip()
            if chromium_executable:
                launch_kwargs["executable_path"] = chromium_executable
            proxy = _playwright_proxy(proxy_url)
            if proxy:
                launch_kwargs["proxy"] = proxy
            browser = playwright_ctx.chromium.launch(**launch_kwargs)
            context = browser.new_context(
                viewport=viewport,
                user_agent=(
                    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
                ),
                locale=locale or "en-US",
                timezone_id=str((browser_cfg or {}).get("timezone") or "America/Chicago"),
                extra_http_headers={"Accept-Language": _accept_language_for_locale(locale)},
            )
            page = context.new_page()
            page.goto(f"{origin}/index.html", wait_until="domcontentloaded", timeout=60000)

            deadline = time.time() + timeout_ms / 1000
            logged_events = 0
            while time.time() < deadline:
                events = bridge_state["events"]
                while logged_events < len(events):
                    event = events[logged_events]
                    logged_events += 1
                    event_type = event.get("type") or "event"
                    tag = event.get("tag") or ""
                    if tag:
                        log(f"[gopay] Stripe invisible payload: {tag}")
                    elif event_type in ("frame_ready", "invisible_initialize", "invisible_execute"):
                        log(f"[gopay] Stripe invisible event: {event_type}")
                if result_event.wait(timeout=1):
                    result = bridge_state.get("result") or {}
                    token = str(result.get("response") or "")
                    ekey = str(result.get("ekey") or "")
                    if token:
                        log(f"[gopay] browser passive hCaptcha solved token_len={len(token)} ekey_len={len(ekey)}")
                        return token, ekey
                    raise GoPayError("approve blocked and browser passive hCaptcha returned empty token")
                if error_event.is_set():
                    err = bridge_state.get("error") or {}
                    raise GoPayError(f"approve blocked and browser passive hCaptcha failed: {str(err)[:240]}")
            raise GoPayError(f"approve blocked and browser passive hCaptcha timeout ({timeout_ms // 1000}s)")
        finally:
            if page is not None:
                try:
                    page.close()
                except Exception:
                    pass
            if browser is not None:
                try:
                    browser.close()
                except Exception:
                    pass
            if playwright_ctx is not None:
                try:
                    playwright_ctx.stop()
                except Exception:
                    pass
            httpd.shutdown()
            httpd.server_close()


def _is_approve_blocked_error(exc: Exception) -> bool:
    text = str(exc).lower()
    return "approve" in text and "blocked" in text


# ──────────────────────────── core ────────────────────────────────


class GoPayCharger:
    """Drive the entire GoPay tokenization flow for one subscription.

    Construction needs:
        chatgpt_session: a requests.Session pre-configured with the user's
            chatgpt.com cookies + sentinel headers. Caller is responsible.
        gopay_cfg: {"country_code": "86", "phone_number": "...", "pin": "..."}
        otp_provider: () -> str. Called once per linking; should block until
            the user supplies the OTP via WhatsApp.
        log: () -> None. Called for human-readable progress messages.
    """

    def __init__(
        self,
        chatgpt_session: Any,
        gopay_cfg: dict,
        otp_provider: Callable[[], str],
        log: Callable[[str], None] = print,
        checkout_proxy: Optional[str] = None,
        payment_proxy: Optional[str] = None,
        runtime_cfg: Optional[dict] = None,
        checkout_cfg: Optional[dict] = None,
        browser_challenge_cfg: Optional[dict] = None,
        pre_solve_passive_captcha: bool = False,
    ):
        self.cs = chatgpt_session
        self.checkout_proxy = _clean_proxy(checkout_proxy)
        self.payment_proxy = _clean_proxy(payment_proxy)
        self.country_code = str(gopay_cfg["country_code"]).lstrip("+")
        self.phone = re.sub(r"\D", "", str(gopay_cfg["phone_number"]))
        self.pin = str(gopay_cfg["pin"])
        self.browser_locale = str(gopay_cfg.get("browser_locale") or "zh-CN")
        self.pin_locale = str(gopay_cfg.get("pin_locale") or "id")
        self.browser_platform = str(gopay_cfg.get("browser_platform") or "Mac OS 10.15.7")
        self.midtrans_client_id = str(
            gopay_cfg.get("midtrans_client_id") or DEFAULT_MIDTRANS_CLIENT_ID
        )
        self.midtrans_tokenization = _normalize_midtrans_tokenization(
            gopay_cfg.get("tokenization")
        )
        self.otp_provider = otp_provider
        self.log = log
        self._midtrans_merchant_id: Optional[str] = None
        # Stripe runtime fingerprint (js_checksum / rv_timestamp / version) — these
        # are computed by Stripe.js client-side; replay the captured values from
        # config.runtime or HAR. Without them confirm 400.
        self.runtime = runtime_cfg or {}
        self.checkout_cfg = checkout_cfg or {}
        self.browser_challenge_cfg = browser_challenge_cfg or {}
        self.pre_solve_passive_captcha = bool(pre_solve_passive_captcha)
        # separate session for non-chatgpt domains (avoid leaking chatgpt cookies)
        self.ext = _new_session()
        self.ext.headers.update({
            "User-Agent": (
                self.cs.headers.get("User-Agent")
                or "Mozilla/5.0 (Macintosh; Intel Mac OS X 12_2_1) "
                "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
            ),
            "Accept-Language": (
                "zh-CN,zh;q=0.9,en;q=0.8"
                if self.browser_locale.lower().startswith("zh")
                else "en-US,en;q=0.9"
            ),
        })
        _set_session_proxy(self.cs, self.payment_proxy)
        _set_session_proxy(self.ext, self.payment_proxy)
        self.checkout_url = ""

    def close(self) -> None:
        for sess in (self.cs, self.ext):
            close = getattr(sess, "close", None)
            if callable(close):
                try:
                    close()
                except Exception:
                    pass

    def _chatgpt_request(self, method: str, url: str, *, proxy: Optional[str] = None, **kwargs: Any) -> Any:
        log = getattr(self, "log", None)
        _set_session_proxy(self.cs, self.payment_proxy if proxy is None else proxy)
        return _request_with_retries(
            self.cs,
            method,
            url,
            log=log if callable(log) else (lambda _msg: None),
            **kwargs,
        )

    def _ext_request(self, method: str, url: str, **kwargs: Any) -> Any:
        log = getattr(self, "log", None)
        _set_session_proxy(self.ext, getattr(self, "payment_proxy", None))
        return _request_with_retries(
            self.ext,
            method,
            url,
            log=log if callable(log) else (lambda _msg: None),
            **kwargs,
        )

    # ───── Step 1-4: ChatGPT/Stripe checkout ─────

    def _chatgpt_create_checkout(self) -> str:
        plan = self.checkout_cfg
        promo_campaign_id = str(plan.get("promo_campaign_id") or "plus-1-month-free").strip()
        body = {
            "entry_point": str(plan.get("entry_point") or "all_plans_pricing_modal"),
            "plan_name": str(plan.get("plan_name") or "chatgptplusplan"),
            "billing_details": {
                "country": str(plan.get("billing_country") or "ID"),
                "currency": str(plan.get("billing_currency") or "IDR"),
            },
            "checkout_ui_mode": str(plan.get("checkout_ui_mode") or "hosted"),
        }
        cancel_url = str(plan.get("cancel_url") or "https://chatgpt.com/#pricing").strip()
        if cancel_url:
            body["cancel_url"] = cancel_url
        if promo_campaign_id:
            body["promo_campaign"] = {
                "promo_campaign_id": promo_campaign_id,
                "is_coupon_from_query_param": False,
            }
        r = self._chatgpt_request(
            "post",
            "https://chatgpt.com/backend-api/payments/checkout",
            json=body, timeout=DEFAULT_TIMEOUT, proxy=self.checkout_proxy,
        )
        if getattr(r, "status_code", 0) >= 400:
            raise GoPayError(f"checkout create failed: status={r.status_code} {_response_excerpt(r)}")
        data = r.json()
        cs_id = _extract_checkout_session_id(data)
        if not cs_id or not str(cs_id).startswith("cs_"):
            raise GoPayError(f"checkout create: bad response {_json_excerpt(data)}")
        self.checkout_url = _checkout_url_from_response(data, cs_id)
        self.log(f"[gopay] checkout created cs_present={'yes' if cs_id else 'no'} hosted_url={'yes' if self.checkout_url else 'no'}")
        return cs_id

    def _stripe_create_pm(self, cs_id: str, stripe_pk: str, billing: dict) -> str:
        # PM billing 即使 IDR 计划也接受 US 地址（HAR 验证）；空配置时给个有效默认
        body = {
            "billing_details[name]": billing.get("name") or "John Doe",
            "billing_details[email]": billing.get("email") or "buyer@example.com",
            "billing_details[address][country]": billing.get("country") or "US",
            "billing_details[address][line1]": billing.get("line1") or "3110 Sunset Boulevard",
            "billing_details[address][city]": billing.get("city") or "Los Angeles",
            "billing_details[address][postal_code]": billing.get("postal_code") or "90026",
            "billing_details[address][state]": billing.get("state") or "CA",
            "type": "gopay",
            "client_attribution_metadata[checkout_session_id]": cs_id,
            "key": stripe_pk,
        }
        r = _request_with_retries(
            self.ext,
            "post",
            "https://api.stripe.com/v1/payment_methods",
            data=body,
            timeout=DEFAULT_TIMEOUT,
            log=self.log,
        )
        r.raise_for_status()
        pm_id = r.json().get("id", "")
        if not pm_id.startswith("pm_"):
            raise GoPayError(f"stripe payment_methods: bad response {_response_excerpt(r, limit=300)}")
        self.log("[gopay] stripe pm=present")
        return pm_id

    def _stripe_init(self, cs_id: str, stripe_pk: str) -> dict:
        """Call /payment_pages/{cs}/init and validate this session supports GoPay."""
        body = {
            "browser_locale": "en-US",
            "browser_timezone": "Asia/Shanghai",
            "elements_session_client[client_betas][0]": "custom_checkout_server_updates_1",
            "elements_session_client[client_betas][1]": "custom_checkout_manual_approval_1",
            "elements_session_client[elements_init_source]": "custom_checkout",
            "elements_session_client[referrer_host]": "chatgpt.com",
            "elements_session_client[stripe_js_id]": str(uuid.uuid4()),
            "elements_session_client[locale]": "en",
            "elements_session_client[is_aggregation_expected]": "false",
            "elements_options_client[stripe_js_locale]": "auto",
            "key": stripe_pk,
        }
        r = _request_with_retries(
            self.ext,
            "post",
            f"https://api.stripe.com/v1/payment_pages/{cs_id}/init",
            data=body,
            timeout=DEFAULT_TIMEOUT,
            log=self.log,
        )
        r.raise_for_status()
        data = r.json() or {}
        pm_types = [pm for pm in data.get("payment_method_types", []) if isinstance(pm, str)]
        currency = str(data.get("currency") or "").lower()
        self.log(f"[gopay] stripe init currency={currency or '?'} payment_method_types={pm_types}")
        if "gopay" not in pm_types:
            raise GoPayError(
                "checkout does not support GoPay: "
                f"currency={currency or '?'} payment_method_types={pm_types}; "
                "need modern hosted IDR checkout",
            )
        ic = data.get("init_checksum") or ""
        if not ic:
            raise GoPayError(f"stripe init: no init_checksum {_response_excerpt(r, limit=200)}")
        return data

    @staticmethod
    def _extract_redirect_to_url(payload: dict) -> str:
        for key in ("next_action", "payment_intent", "setup_intent"):
            obj = payload.get(key)
            if not isinstance(obj, dict):
                continue
            action = obj if key == "next_action" else obj.get("next_action")
            if isinstance(action, dict) and action.get("type") == "redirect_to_url":
                return ((action.get("redirect_to_url") or {}).get("url") or "").strip()
        return ""

    def _solve_passive_confirm_captcha(self, init_data: dict) -> tuple[str, str]:
        captcha = _extract_passive_captcha_config(init_data)
        self.log(
            "[gopay] passive captcha "
            f"site_key={captcha['site_key'][:12]} rqdata={'yes' if captcha.get('rqdata') else 'no'}"
        )
        browser_cfg = dict(self.browser_challenge_cfg or {})
        payment_proxy = getattr(self, "payment_proxy", "")
        if payment_proxy and not (browser_cfg.get("passive_proxy_url") or browser_cfg.get("proxy_url")):
            browser_cfg["passive_proxy_url"] = payment_proxy
        return _solve_passive_hcaptcha_in_browser(
            captcha,
            browser_cfg=browser_cfg,
            merchant_id=self._midtrans_merchant_id or "",
            locale=self.browser_locale,
            log=self.log,
        )

    def _stripe_confirm(
        self,
        cs_id: str,
        pm_id: str,
        stripe_pk: str,
        *,
        force_passive_captcha: bool = False,
        captcha_token: str = "",
        captcha_ekey: str = "",
    ) -> dict:
        init_data = self._stripe_init(cs_id, stripe_pk)
        init_checksum = init_data.get("init_checksum", "")
        expected_amount, expected_amount_source = _resolve_expected_amount(
            init_data,
            self.runtime,
        )
        self.log(
            f"[gopay] stripe expected_amount={expected_amount} "
            f"source={expected_amount_source}"
        )
        # Stripe 需要 return_url 才会把 checkout 推进到 requires_action（带 setup_intent）
        chatgpt_return = (
            f"https://chatgpt.com/checkout/verify?stripe_session_id={cs_id}"
            f"&processor_entity=openai_llc&plan_type=plus"
        )
        from urllib.parse import quote
        return_url = (
            f"https://checkout.stripe.com/c/pay/{cs_id}"
            f"?returned_from_redirect=true&ui_mode=custom&return_url={quote(chatgpt_return, safe='')}"
        )
        body = {
            "guid": uuid.uuid4().hex,
            "muid": uuid.uuid4().hex,
            "sid": uuid.uuid4().hex,
            "payment_method": pm_id,
            "init_checksum": init_checksum,
            "version": self.runtime.get("version") or "fed52f3bc6",
            "expected_amount": expected_amount,
            "expected_payment_method_type": "gopay",
            "return_url": return_url,
            "elements_session_client[session_id]": f"elements_session_{uuid.uuid4().hex[:11]}",
            "elements_session_client[locale]": "en",
            "elements_session_client[referrer_host]": "chatgpt.com",
            "elements_session_client[is_aggregation_expected]": "false",
            "client_attribution_metadata[client_session_id]": str(uuid.uuid4()),
            "client_attribution_metadata[merchant_integration_source]": "elements",
            "client_attribution_metadata[merchant_integration_subtype]": "payment-element",
            "client_attribution_metadata[payment_intent_creation_flow]": "deferred",
            "key": stripe_pk,
        }
        if force_passive_captcha and not captcha_token:
            captcha_token, captcha_ekey = self._solve_passive_confirm_captcha(init_data)
        if captcha_token:
            body["passive_captcha_token"] = captcha_token
        if captcha_ekey:
            body["passive_captcha_ekey"] = captcha_ekey
        # Stripe runtime anti-bot tokens (replayable per-session-only; without
        # these confirm fails for hCaptcha-protected merchants like OpenAI).
        if self.runtime.get("js_checksum"):
            body["js_checksum"] = self.runtime["js_checksum"]
        if self.runtime.get("rv_timestamp"):
            body["rv_timestamp"] = self.runtime["rv_timestamp"]
        r = _request_with_retries(
            self.ext,
            "post",
            f"https://api.stripe.com/v1/payment_pages/{cs_id}/confirm",
            data=body,
            timeout=DEFAULT_TIMEOUT,
            log=self.log,
        )
        if (
            r.status_code == 400
            and "terms of service" in (r.text or "").lower()
            and "consent[terms_of_service]" not in body
        ):
            self.log("[gopay] Stripe confirm requires ToS consent; retrying once")
            body["consent[terms_of_service]"] = "accepted"
            r = _request_with_retries(
                self.ext,
                "post",
                f"https://api.stripe.com/v1/payment_pages/{cs_id}/confirm",
                data=body,
                timeout=DEFAULT_TIMEOUT,
                log=self.log,
            )
        if r.status_code != 200:
            detail = _stripe_confirm_error_detail(
                r.text,
                expected_amount=expected_amount,
                expected_amount_source=expected_amount_source,
            )
            if detail:
                raise GoPayError(detail)
            raise GoPayError(f"stripe confirm {r.status_code}: {_response_excerpt(r, limit=400)}")
        data = r.json() or {}
        self.log(
            f"[gopay] stripe confirm: payment_status={data.get('payment_status')} "
            f"setup_intent_status={(data.get('setup_intent') or {}).get('status')}"
        )
        return data

    def _chatgpt_sentinel_ping(self):
        try:
            self._chatgpt_request(
                "post",
                "https://chatgpt.com/backend-api/sentinel/ping",
                json={}, timeout=DEFAULT_TIMEOUT,
            )
        except Exception as e:
            self.log(f"[gopay] sentinel/ping skipped: {_redact_text_for_log(e)}")

    def _chatgpt_approve(self, cs_id: str, processor_entity: str = "openai_llc"):
        # sentinel/ping 在 approve 之前刷一下，否则 approve 过但 setup_intent 不创
        self._chatgpt_sentinel_ping()
        r = self._chatgpt_request(
            "post",
            "https://chatgpt.com/backend-api/payments/checkout/approve",
            json={"checkout_session_id": cs_id, "processor_entity": processor_entity},
            headers={
                "x-openai-target-path": "/backend-api/payments/checkout/approve",
                "x-openai-target-route": "/backend-api/payments/checkout/approve",
            },
            timeout=DEFAULT_TIMEOUT,
        )
        r.raise_for_status()
        result = r.json().get("result")
        if result != "approved":
            raise GoPayError(f"chatgpt approve: result={result!r}")
        self.log("[gopay] chatgpt approved")

    # ───── Step 5-6: Stripe → Midtrans redirect ─────

    def _follow_redirect_to_midtrans(self, cs_id: str, stripe_pk: str) -> str:
        """Resolve the Midtrans snap_token from setup_intent.next_action.

        After approve, Stripe populates setup_intent on the checkout session.
        The frontend re-GETs payment_pages/{cs} to read
        setup_intent.next_action.redirect_to_url.url which is
        https://pm-redirects.stripe.com/authorize/{acct}/{nonce}. GETting
        that URL with redirects disabled returns 302 → app.midtrans.com/...
        whose path contains the snap_token.
        """
        deadline = time.time() + 60
        last_err = ""
        sess_id = f"elements_session_{uuid.uuid4().hex[:11]}"
        js_id = str(uuid.uuid4())
        params = {
            "elements_session_client[client_betas][0]": "custom_checkout_server_updates_1",
            "elements_session_client[client_betas][1]": "custom_checkout_manual_approval_1",
            "elements_session_client[elements_init_source]": "custom_checkout",
            "elements_session_client[referrer_host]": "chatgpt.com",
            "elements_session_client[session_id]": sess_id,
            "elements_session_client[stripe_js_id]": js_id,
            "elements_session_client[locale]": "en",
            "elements_session_client[is_aggregation_expected]": "false",
            "elements_options_client[stripe_js_locale]": "auto",
            "elements_options_client[saved_payment_method][enable_save]": "never",
            "elements_options_client[saved_payment_method][enable_redisplay]": "never",
            "key": stripe_pk,
            "_stripe_version": (
                "2025-03-31.basil; checkout_server_update_beta=v1; "
                "checkout_manual_approval_preview=v1"
            ),
        }
        while time.time() < deadline:
            r = self._ext_request(
                "get",
                f"https://api.stripe.com/v1/payment_pages/{cs_id}",
                params=params,
                timeout=DEFAULT_TIMEOUT,
            )
            if r.status_code == 200:
                payload = r.json() or {}
                si = payload.get("setup_intent") or {}
                if si.get("status") == "requires_action":
                    rtu = (si.get("next_action") or {}).get("redirect_to_url") or {}
                    pm_url = rtu.get("url") or ""
                    if pm_url:
                        snap_token = self._fetch_pm_redirect_snap_token(pm_url)
                        self.log("[gopay] midtrans snap_token=present")
                        return snap_token
                last_err = (
                    f"setup_intent status={si.get('status')!r} "
                    f"payment_status={payload.get('payment_status')!r} "
                    f"status={payload.get('status')!r} "
                    f"keys=[{','.join(sorted(payload.keys())[:8])}]"
                )
            else:
                last_err = f"http {r.status_code}: {_response_excerpt(r, limit=120)}"
            time.sleep(1)
        raise GoPayError(f"snap_token resolution timeout: {last_err}")

    def _fetch_pm_redirect_snap_token(self, pm_url: str) -> str:
        """GET pm-redirects.stripe.com/authorize/... → 302 to midtrans.
        Extract snap_token from the Location header.
        """
        direct = re.search(
            r"app\.midtrans\.com/snap/v[14]/redirection/([a-f0-9-]{36})",
            pm_url,
        )
        if direct:
            return direct.group(1)
        r = self._ext_request("get", pm_url, allow_redirects=False, timeout=DEFAULT_TIMEOUT)
        if r.status_code not in (301, 302, 303, 307, 308):
            raise GoPayError(f"pm-redirects: expected redirect, got {r.status_code}")
        loc = r.headers.get("Location", "")
        m = re.search(r"app\.midtrans\.com/snap/v[14]/redirection/([a-f0-9-]{36})", loc)
        if not m:
            raise GoPayError(f"pm-redirects: no midtrans token in Location={_redact_text_for_log(loc)!r}")
        return m.group(1)

    def _midtrans_load_transaction(self, snap_token: str):
        """Seed Midtrans cookies, then load transaction metadata."""
        redirection_url = self._midtrans_redirection_url(snap_token)
        try:
            landing = self._ext_request(
                "get",
                redirection_url,
                headers={
                    "Accept": (
                        "text/html,application/xhtml+xml,application/xml;q=0.9,"
                        "image/avif,image/webp,image/apng,*/*;q=0.8"
                    ),
                    "Referer": "https://pay.openai.com/",
                },
                timeout=DEFAULT_TIMEOUT,
            )
            if landing.status_code >= 400:
                self.log(f"[gopay] midtrans redirection warmup status={landing.status_code}")
        except Exception as e:
            self.log(f"[gopay] midtrans redirection warmup skipped: {_redact_text_for_log(e)}")

        try:
            self.ext.cookies.set("locale", "en", domain="app.midtrans.com", path="/")
        except Exception:
            pass

        r = self._ext_request(
            "get",
            f"https://app.midtrans.com/snap/v1/transactions/{snap_token}",
            headers=self._midtrans_headers(snap_token, source=True),
            timeout=DEFAULT_TIMEOUT,
        )
        r.raise_for_status()
        body = r.json()
        merchant = body.get("merchant") or {}
        merchant_id = merchant.get("merchant_id") or ""
        if merchant_id:
            self._midtrans_merchant_id = merchant_id
            try:
                self.ext.cookies.set(
                    f"preferredPayment-{merchant_id}",
                    "gopay",
                    domain="app.midtrans.com",
                    path="/",
                )
            except Exception:
                pass
        enabled = [p.get("type") for p in body.get("enabled_payments", [])]
        self.log(f"[gopay] midtrans enabled_payments={enabled}")
        self._midtrans_warm_snap_side_effects(snap_token)

    def _midtrans_warm_snap_side_effects(self, snap_token: str):
        """Replay non-critical Snap XHRs seen before linking in the browser."""
        try:
            self._ext_request(
                "post",
                f"https://app.midtrans.com/snap/v1/promos/{snap_token}/search",
                headers=self._midtrans_headers(snap_token, source=True, origin=True),
                timeout=DEFAULT_TIMEOUT,
            )
        except Exception as e:
            self.log(f"[gopay] midtrans promos warmup skipped: {_redact_text_for_log(e)}")
        try:
            self._ext_request(
                "get",
                "https://app.midtrans.com/snap/v3/experiment",
                params={"id": snap_token},
                headers=self._midtrans_headers(snap_token, source=True),
                timeout=DEFAULT_TIMEOUT,
            )
        except Exception as e:
            self.log(f"[gopay] midtrans experiment warmup skipped: {_redact_text_for_log(e)}")

    def _midtrans_basic_auth(self) -> dict:
        import base64
        token = base64.b64encode(
            f"{self.midtrans_client_id}:".encode("ascii"),
        ).decode("ascii")
        return {"Authorization": f"Basic {token}"}

    @staticmethod
    def _midtrans_redirection_url(snap_token: str) -> str:
        return f"https://app.midtrans.com/snap/v4/redirection/{snap_token}"

    def _midtrans_headers(
        self,
        snap_token: str,
        *,
        json_body: bool = False,
        source: bool = False,
        auth: bool = False,
        origin: bool = False,
    ) -> dict:
        headers = {
            "Accept": "application/json",
            "Referer": self._midtrans_redirection_url(snap_token),
        }
        if json_body:
            headers["Content-Type"] = "application/json"
            origin = True
        if origin:
            headers["Origin"] = "https://app.midtrans.com"
        if source:
            headers.update({
                "x-source": "snap",
                "x-source-app-type": "redirection",
                "x-source-version": "2.3.0",
            })
        if auth:
            headers.update(self._midtrans_basic_auth())
        return headers

    # ───── Step 7: Midtrans linking initiation ─────

    def _midtrans_init_linking(self, snap_token: str) -> str:
        """POST snap/v3/accounts/{snap}/linking. Retries on 406, bypasses on 429."""
        url = f"https://app.midtrans.com/snap/v3/accounts/{snap_token}/linking"
        body = {
            "type": "gopay",
            "country_code": self.country_code,
            "phone_number": self.phone,
        }
        base_headers = self._midtrans_headers(snap_token, json_body=True)
        auth_headers = self._midtrans_headers(snap_token, json_body=True, auth=True)
        last_err: Optional[str] = None
        bypass_tried = False
        for attempt in range(1, LINK_RETRY_LIMIT + 2):
            r = self._ext_request("post", url, json=body, headers=auth_headers, timeout=DEFAULT_TIMEOUT)
            ref = self._parse_linking_reference(r)
            if ref:
                self.log(f"[gopay] midtrans linking ok reference={ref}")
                return ref
            if r.status_code == 406:
                try:
                    j = r.json()
                except Exception:
                    j = None
                if isinstance(j, dict):
                    last_err = (j.get("error_messages") or ["?"])[0]
                elif isinstance(j, list) and j:
                    last_err = str(j[0])
                else:
                    last_err = _response_excerpt(r, limit=120)
                self.log(f"[gopay] midtrans linking 406 ({last_err}), 冷却 {LINK_RETRY_SLEEP_S}s 再重试 {attempt}/{LINK_RETRY_LIMIT}")
                time.sleep(LINK_RETRY_SLEEP_S)
                continue
            if not bypass_tried and self._linking_is_rate_limited(r):
                bypass_tried = True
                self.log(
                    f"[gopay] midtrans linking rate-limited status={r.status_code}; retrying without Authorization",
                )
                rb = self._ext_request(
                    "post",
                    url, json=body, headers=base_headers, timeout=DEFAULT_TIMEOUT,
                )
                ref = self._parse_linking_reference(rb)
                if ref:
                    self.log(f"[gopay] midtrans linking bypass ok reference={ref}")
                    return ref
                raise GoPayError(
                    f"midtrans linking bypass failed status={rb.status_code} body={_response_excerpt(rb, limit=300)}",
                )
            raise GoPayError(
                f"midtrans linking unexpected status={r.status_code} body={_response_excerpt(r, limit=300)}",
            )
        raise GoPayError(f"midtrans linking exhausted retries: {last_err}")

    @staticmethod
    def _parse_linking_reference(r) -> Optional[str]:
        if r.status_code != 201:
            return None
        try:
            data = r.json()
        except Exception:
            return None
        m = re.search(r"reference=([a-f0-9-]{36})", data.get("activation_link_url", ""))
        if not m:
            raise GoPayError(f"midtrans linking 201 but no reference: {_json_excerpt(data)}")
        return m.group(1)

    @staticmethod
    def _linking_is_rate_limited(r) -> bool:
        if r.status_code == 429:
            return True
        text = (r.text or "").lower()
        return any(h in text for h in LINK_BYPASS_BODY_HINTS)

    # ───── Step 8-12: GoPay linking ─────

    def _gopay_headers(
        self,
        *,
        json_body: bool = True,
        locale: Optional[str] = None,
        origin: str = "https://merchants-gws-app.gopayapi.com",
        referer: str = "https://merchants-gws-app.gopayapi.com/",
    ) -> dict:
        headers = {
            "Accept": "application/json, text/plain, */*",
            "Origin": origin,
            "Referer": referer,
        }
        if json_body:
            headers["Content-Type"] = "application/json"
        if locale:
            headers["x-user-locale"] = locale
        return headers

    def _gopay_validate_reference(self, reference_id: str):
        r = self._ext_request(
            "post",
            "https://gwa.gopayapi.com/v1/linking/validate-reference",
            json={"reference_id": reference_id},
            headers=self._gopay_headers(locale=None),
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code != 200:
            raise GoPayError(f"validate-reference {r.status_code}: {_response_excerpt(r)}")
        if not r.json().get("success"):
            raise GoPayError(f"validate-reference failed: {_response_excerpt(r)}")

    def _gopay_user_consent(self, reference_id: str):
        r = self._ext_request(
            "post",
            "https://gwa.gopayapi.com/v1/linking/user-consent",
            json={"reference_id": reference_id},
            headers=self._gopay_headers(locale=self.browser_locale),
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code != 200:
            raise GoPayError(f"user-consent {r.status_code}: {_response_excerpt(r)}")
        payload = r.json()
        if not payload.get("success"):
            raise GoPayError(f"user-consent failed: {_response_excerpt(r)}")
        if _linking_consent_requires_otp(payload):
            self.log("[gopay] consent ok, OTP required")
        else:
            self.log("[gopay] consent ok, OTP not required")
        return payload

    def _gopay_resend_otp(self, reference_id: str) -> dict:
        """Try resend-otp to trigger SMS delivery."""
        try:
            r = self._ext_request(
                "post",
                "https://gwa.gopayapi.com/v1/linking/resend-otp",
                json={"reference_id": reference_id},
                headers=self._gopay_headers(locale=self.browser_locale),
                timeout=DEFAULT_TIMEOUT,
            )
            self.log(f"[gopay] resend-otp status={r.status_code} body={_response_excerpt(r, limit=200)}")
            return {
                "success": 200 <= int(r.status_code) < 300,
                "status_code": int(r.status_code),
                "response_text": _response_excerpt(r, limit=400),
            }
        except Exception as e:
            self.log(f"[gopay] resend-otp failed: {_redact_text_for_log(e)}")
            return {
                "success": False,
                "error_message": str(e),
            }

    def _gopay_validate_otp(self, reference_id: str, otp: str) -> tuple[str, str]:
        """Returns (challenge_id, client_id) for PIN tokenization."""
        r = self._ext_request(
            "post",
            "https://gwa.gopayapi.com/v1/linking/validate-otp",
            json={"reference_id": reference_id, "otp": otp},
            headers=self._gopay_headers(locale=self.browser_locale),
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code != 200:
            message = f"validate-otp {r.status_code}: {_response_excerpt(r, limit=400)}"
            if r.status_code == 400:
                raise GoPayOTPRejected(message)
            raise GoPayError(message)
        data = r.json()
        if not data.get("success"):
            raise GoPayOTPRejected(f"validate-otp failed: {_json_excerpt(data)}")
        challenge = (
            data.get("data", {}).get("challenge", {}).get("action", {}).get("value", {})
        )
        challenge_id = challenge.get("challenge_id") or ""
        client_id = challenge.get("client_id") or ""
        if not challenge_id or not client_id:
            raise GoPayError(f"validate-otp: missing challenge details {_json_excerpt(data)}")
        self.log(f"[gopay] otp ok challenge_id={challenge_id[:8]}…")
        return challenge_id, client_id

    def _tokenize_pin(self, challenge_id: str, client_id: str, *, purpose: str) -> str:
        """POST customer.gopayapi.com/api/v1/users/pin/tokens/nb → JWT."""
        if purpose == "linking":
            headers = self._gopay_headers(
                locale=self.pin_locale,
                origin="https://pin-web-client.gopayapi.com",
                referer="https://pin-web-client.gopayapi.com/",
            )
            headers.update({
                "x-appversion": "1.0.0",
                "x-correlation-id": str(uuid.uuid4()),
                "x-is-mobile": "false",
                "x-platform": self.browser_platform,
                "x-request-id": str(uuid.uuid4()),
            })
            body = {
                "challenge_id": challenge_id,
                "client_id": client_id,
                "pin": self.pin,
            }
        elif purpose == "payment":
            headers = self._gopay_headers(locale=None)
            headers["x-request-id"] = str(uuid.uuid4())
            body = {
                "pin": self.pin,
                "challenge_id": challenge_id,
                "client_id": client_id,
            }
        else:
            raise GoPayError(f"unknown pin token purpose={purpose!r}")
        r = self._ext_request(
            "post",
            "https://customer.gopayapi.com/api/v1/users/pin/tokens/nb",
            json=body,
            headers=headers,
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code in (400, 401, 403):
            raise GoPayPINRejected(f"PIN rejected: {_response_excerpt(r, limit=300)}")
        if r.status_code >= 400:
            raise GoPayError(f"pin tokenize {r.status_code}: {_response_excerpt(r)}")
        body = r.json() if r.headers.get("content-type", "").startswith("application/json") else {}
        # Token can be in different shapes; check common keys
        token = (
            body.get("token")
            or body.get("data", {}).get("token")
            or body.get("data", {}).get("pin_token")
            or ""
        )
        if not token:
            # Some flows return the JWT in a wrapper; check for raw redirect URL
            # hash extraction not needed since the JWT is in the body for /nb endpoints
            raise GoPayError(f"pin tokenize: no token in response {_response_excerpt(r, limit=300)}")
        return token

    def _gopay_validate_pin(self, reference_id: str, pin_token: str):
        r = self._ext_request(
            "post",
            "https://gwa.gopayapi.com/v1/linking/validate-pin",
            json={"reference_id": reference_id, "token": pin_token},
            headers=self._gopay_headers(locale=self.browser_locale),
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code != 200:
            raise GoPayError(f"validate-pin {r.status_code}: {_response_excerpt(r)}")
        if not r.json().get("success"):
            raise GoPayError(f"validate-pin failed: {_response_excerpt(r)}")
        self.log("[gopay] linking complete")

    # ───── Step 13: Midtrans charge initiation ─────

    def _midtrans_create_charge_data(self, snap_token: str) -> dict:
        """POST snap/v2/transactions/{snap}/charge and keep user-facing links."""
        url = f"https://app.midtrans.com/snap/v2/transactions/{snap_token}/charge"
        headers = self._midtrans_headers(snap_token, json_body=True, source=True)
        r = self._ext_request(
            "post",
            url,
            json={
                "payment_type": "gopay",
                "tokenization": self.midtrans_tokenization,
                "promo_details": None,
            },
            headers=headers, timeout=DEFAULT_TIMEOUT,
        )
        r.raise_for_status()
        data = r.json()
        self.log("[gopay] midtrans charge response " + _json_excerpt(data, limit=1200))
        denied = _midtrans_charge_denial_message(data)
        if denied:
            raise GoPayError(denied)
        charge_ref = _extract_midtrans_charge_reference(data)
        if not charge_ref:
            raise GoPayError(f"midtrans charge: no reference in response {_json_excerpt(data)}")
        self.log(f"[gopay] midtrans charge ref={charge_ref}")
        charge_data = {
            "charge_ref": charge_ref,
            "snap_token": snap_token,
        }
        charge_data.update(_midtrans_charge_urls(data))
        if self.midtrans_tokenization.lower() == "false":
            for key in ("deeplink_url", "qr_code_url", "finish_redirect_url", "finish_200_redirect_url"):
                value = charge_data.get(key, "")
                if value:
                    self.log(f"[gopay] midtrans {key}=present")
        return charge_data

    def _midtrans_create_charge(self, snap_token: str) -> str:
        """POST snap/v2/transactions/{snap}/charge → charge_ref like A12..."""
        return str(self._midtrans_create_charge_data(snap_token).get("charge_ref") or "")

    def _midtrans_poll_status(self, snap_token: str) -> dict:
        """Poll Snap transaction status until GoPay settlement is visible."""
        url = f"https://app.midtrans.com/snap/v1/transactions/{snap_token}/status"
        last = ""
        for _ in range(MIDTRANS_STATUS_POLL_LIMIT):
            r = self._ext_request(
                "get",
                url,
                headers=self._midtrans_headers(snap_token, source=True),
                timeout=DEFAULT_TIMEOUT,
            )
            if r.status_code == 200:
                data = r.json()
                status = str(data.get("transaction_status") or "")
                status_code = str(data.get("status_code") or "")
                last = f"status={status!r} status_code={status_code!r}"
                if status in {"settlement", "capture"} or status_code == "200":
                    self.log(f"[gopay] midtrans status ok {last}")
                    return data
                if status in {"deny", "cancel", "expire", "failure"}:
                    raise GoPayError(f"midtrans transaction failed: {_json_excerpt(data)}")
            else:
                last = f"http {r.status_code}: {_response_excerpt(r, limit=120)}"
            time.sleep(2)
        self.log(f"[gopay] midtrans status poll timeout: {last}")
        return {}

    def _midtrans_finish_redirect_url(self, state: dict, midtrans_status: dict) -> str:
        for source in (midtrans_status, state):
            if not isinstance(source, dict):
                continue
            for key in ("finish_redirect_url", "finish_200_redirect_url"):
                value = str(source.get(key) or "").strip()
                if value:
                    return value
        return ""

    def _follow_midtrans_finish_redirect(self, state: dict, midtrans_status: dict) -> str:
        finish_url = self._midtrans_finish_redirect_url(state, midtrans_status)
        if not finish_url:
            self.log("[gopay] midtrans finish redirect skipped: no URL")
            return ""
        try:
            r = self._ext_request(
                "get",
                finish_url,
                headers={
                    "Accept": (
                        "text/html,application/xhtml+xml,application/xml;q=0.9,"
                        "image/avif,image/webp,image/apng,*/*;q=0.8"
                    ),
                    "Referer": self._midtrans_redirection_url(str(state.get("snap_token") or "")),
                },
                allow_redirects=True,
                timeout=DEFAULT_TIMEOUT,
            )
            self.log(f"[gopay] midtrans finish redirect status={r.status_code}")
        except Exception as exc:
            self.log(f"[gopay] midtrans finish redirect failed: {_redact_text_for_log(exc)[:240]}")
        return finish_url

    # ───── Step 14: GoPay charge processing ─────

    def _gopay_payment_validate(self, charge_ref: str):
        # midtrans 创建 charge 后 GoPay 后端要数秒才能 fetch；轮询直到 ready
        for i in range(8):
            r = self._ext_request(
                "get",
                f"https://gwa.gopayapi.com/v1/payment/validate?reference_id={charge_ref}",
                headers=self._gopay_headers(json_body=False),
                timeout=DEFAULT_TIMEOUT,
            )
            if r.status_code == 200 and r.json().get("success"):
                return
            time.sleep(1.5)
        raise GoPayError(f"payment/validate failed after retries: {r.status_code} {_response_excerpt(r, limit=200)}")

    def _gopay_payment_confirm(self, charge_ref: str) -> tuple[str, str]:
        """Returns (challenge_id, client_id) for the charge PIN."""
        r = self._ext_request(
            "post",
            f"https://gwa.gopayapi.com/v1/payment/confirm?reference_id={charge_ref}",
            json={"payment_instructions": []},
            headers=self._gopay_headers(locale=None),
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code != 200:
            raise GoPayError(f"payment/confirm {r.status_code}: {_response_excerpt(r)}")
        data = r.json()
        if not data.get("success"):
            raise GoPayError(f"payment/confirm failed: {_json_excerpt(data)}")
        ch = data.get("data", {}).get("challenge", {}).get("action", {}).get("value", {})
        return ch.get("challenge_id", ""), ch.get("client_id", "")

    def _gopay_payment_process(self, charge_ref: str, pin_token: str):
        r = self._ext_request(
            "post",
            f"https://gwa.gopayapi.com/v1/payment/process?reference_id={charge_ref}",
            json={
                "challenge": {
                    "type": "GOPAY_PIN_CHALLENGE",
                    "value": {"pin_token": pin_token},
                },
            },
            headers=self._gopay_headers(locale=None),
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code != 200:
            raise GoPayError(f"payment/process {r.status_code}: {_response_excerpt(r)}")
        data = r.json()
        if not data.get("success") or data.get("data", {}).get("next_action") != "payment-success":
            raise GoPayError(f"payment/process failed: {_json_excerpt(data)}")
        self.log("[gopay] charge settled")

    # ───── Step 15: Stripe + ChatGPT verify ─────

    def _chatgpt_verify(self, cs_id: str) -> dict:
        """Poll chatgpt verify until plan is active."""
        deadline = time.time() + 60
        while time.time() < deadline:
            r = self._chatgpt_request(
                "get",
                "https://chatgpt.com/checkout/verify",
                params={
                    "stripe_session_id": cs_id,
                    "processor_entity": "openai_llc",
                    "plan_type": "plus",
                },
                timeout=DEFAULT_TIMEOUT,
                allow_redirects=True,
            )
            if r.status_code == 200:
                self.log("[gopay] chatgpt verify ok")
                return {"state": "succeeded", "cs_id": cs_id}
            time.sleep(2)
        return {"state": "verify_timeout", "cs_id": cs_id}

    # ───── Top-level driver ─────

    def run(self, stripe_pk: str, billing: Optional[dict] = None) -> dict:
        state = self.start_until_otp(stripe_pk, billing=billing)
        otp = self.otp_provider()
        return self.complete_after_otp(state, otp)

    def run_from_redirect(
        self, pm_redirect_url: str, cs_id: str = "", stripe_pk: str = "",
    ) -> dict:
        """半自动模式：用户在浏览器走到 pm-redirects.stripe.com 那一步，把
        URL 粘过来；gopay 接管 Midtrans linking + OTP + PIN + 扣款 + verify。
        """
        snap_token = self._fetch_pm_redirect_snap_token(pm_redirect_url)
        self.log("[gopay] midtrans snap_token=present")
        return self._run_midtrans_and_gopay(snap_token, cs_id, stripe_pk)

    def start_until_otp(
        self,
        stripe_pk: str,
        billing: Optional[dict] = None,
        checkout_session_id: str = "",
        checkout_url: str = "",
        otp_channel: str = "",
    ) -> dict:
        """Run checkout/linking until GoPay has sent the requested OTP."""
        prepared = self.prepare_until_linking(
            stripe_pk,
            billing=billing,
            checkout_session_id=checkout_session_id,
            checkout_url=checkout_url,
        )
        return self.start_prepared_linking_until_otp(prepared, otp_channel=otp_channel)

    def prepare_until_linking(
        self,
        stripe_pk: str,
        billing: Optional[dict] = None,
        checkout_session_id: str = "",
        checkout_url: str = "",
    ) -> dict:
        """Run checkout/Stripe/Midtrans redirect setup, but do not trigger GoPay linking."""
        billing = billing or {}
        checkout_url = str(checkout_url or "").strip()
        cs_id = str(checkout_session_id or "").strip()
        if not cs_id and checkout_url:
            cs_id = _extract_checkout_session_id({"url": checkout_url})
        if checkout_url and not cs_id:
            raise GoPayError("checkout_url does not contain checkout_session_id")
        if cs_id:
            if not cs_id.startswith("cs_"):
                raise GoPayError(f"invalid checkout_session_id: {cs_id}")
            self.checkout_url = checkout_url or f"https://checkout.stripe.com/c/pay/{cs_id}"
            self.log("[gopay] reusing checkout session from probe")
        else:
            cs_id = self._chatgpt_create_checkout()
        pm_id = self._stripe_create_pm(cs_id, stripe_pk, billing)
        confirm_data = self._stripe_confirm(
            cs_id,
            pm_id,
            stripe_pk,
            force_passive_captcha=self.pre_solve_passive_captcha,
        )
        redirect_url = self._extract_redirect_to_url(confirm_data)
        if redirect_url:
            self.log("[gopay] confirm returned redirect directly")
            snap_token = self._fetch_pm_redirect_snap_token(redirect_url)
        else:
            try:
                self._chatgpt_approve(cs_id)
                snap_token = self._follow_redirect_to_midtrans(cs_id, stripe_pk)
            except GoPayError as exc:
                if not _is_approve_blocked_error(exc):
                    raise
                self.log("[gopay] chatgpt approve blocked; retrying confirm with passive hCaptcha")
                pm_id = self._stripe_create_pm(cs_id, stripe_pk, billing)
                confirm_data = self._stripe_confirm(
                    cs_id,
                    pm_id,
                    stripe_pk,
                    force_passive_captcha=True,
                )
                redirect_url = self._extract_redirect_to_url(confirm_data)
                if redirect_url:
                    self.log("[gopay] captcha confirm returned redirect directly")
                    snap_token = self._fetch_pm_redirect_snap_token(redirect_url)
                else:
                    self._chatgpt_approve(cs_id)
                    snap_token = self._follow_redirect_to_midtrans(cs_id, stripe_pk)
        self.log("[gopay] midtrans snap_token=present")
        return {
            "state": "prepared",
            "cs_id": cs_id,
            "checkout_url": getattr(self, "checkout_url", ""),
            "stripe_pk": stripe_pk,
            "snap_token": snap_token,
        }

    def start_prepared_linking_until_otp(
        self,
        state: dict,
        otp_channel: str = "",
        gopay_phone: str = "",
    ) -> dict:
        """Trigger GoPay user-linking from a prepared Midtrans snap state."""
        snap_token = str(state.get("snap_token") or "").strip()
        cs_id = str(state.get("cs_id") or state.get("checkout_session_id") or "").strip()
        stripe_pk = str(state.get("stripe_pk") or "").strip()
        checkout_url = str(state.get("checkout_url") or "").strip()
        if not snap_token:
            raise GoPayError("prepared payment is missing snap_token")
        if checkout_url:
            self.checkout_url = checkout_url
        phone = re.sub(r"\D", "", str(gopay_phone or "").strip())
        if phone:
            self.phone = phone
        if not getattr(self, "phone", ""):
            raise GoPayError("gopay_phone is required before linking")
        return self.start_linking_until_otp(snap_token, cs_id, stripe_pk, otp_channel=otp_channel)

    def resend_linking_otp(self, state: dict) -> dict:
        reference_id = str(state.get("reference_id") or "").strip()
        if not reference_id:
            raise GoPayError("prepared payment is missing reference_id")
        result = self._gopay_resend_otp(reference_id)
        if not result.get("success"):
            message = str(result.get("error_message") or result.get("response_text") or "resend-otp failed")
            raise GoPayError(message)
        next_state = dict(state)
        next_state["issued_after_unix"] = int(time.time())
        next_state["otp_resend_count"] = int(next_state.get("otp_resend_count") or 0) + 1
        next_state["otp_required"] = True
        return next_state

    def start_linking_until_otp(
        self, snap_token: str, cs_id: str = "", stripe_pk: str = "", otp_channel: str = "",
    ) -> dict:
        """Load Midtrans, trigger GoPay linking OTP, and return resumable state."""
        otp_channel = _normalize_otp_channel(otp_channel)
        self._midtrans_load_transaction(snap_token)
        reference_id = self._midtrans_init_linking(snap_token)
        self._gopay_validate_reference(reference_id)
        issued_after_unix = int(time.time())
        consent = self._gopay_user_consent(reference_id)
        otp_required = _linking_consent_requires_otp(consent)
        if otp_required and otp_channel == "sms":
            self._gopay_resend_otp(reference_id)
        if not otp_required:
            charge_data = self._midtrans_create_charge_data(snap_token)
            state = {
                "cs_id": cs_id,
                "checkout_url": self.checkout_url,
                "stripe_pk": stripe_pk,
                "snap_token": snap_token,
                "reference_id": reference_id,
                "issued_after_unix": issued_after_unix,
                "otp_channel": otp_channel,
                "otp_required": False,
                "state": "awaiting_manual_confirmation",
            }
            state.update(charge_data)
            return state
        return {
            "cs_id": cs_id,
            "checkout_url": self.checkout_url,
            "stripe_pk": stripe_pk,
            "snap_token": snap_token,
            "reference_id": reference_id,
            "issued_after_unix": issued_after_unix,
            "otp_channel": otp_channel,
            "otp_required": True,
        }

    def complete_after_otp_until_manual_confirmation(self, state: dict, otp: str) -> dict:
        """Resume after OTP, create the Midtrans charge, then wait for manual pay."""
        reference_id = str(state.get("reference_id") or "")
        snap_token = str(state.get("snap_token") or "")
        if not reference_id or not snap_token:
            raise GoPayError("payment flow state is missing reference_id/snap_token")
        otp = (otp or "").strip()
        if not otp:
            raise OTPCancelled("OTP not provided")

        challenge_id, client_id = self._gopay_validate_otp(reference_id, otp)
        pin_token = self._tokenize_pin(challenge_id, client_id, purpose="linking")
        self._gopay_validate_pin(reference_id, pin_token)

        charge_data = self._midtrans_create_charge_data(snap_token)
        next_state = dict(state)
        next_state.update(charge_data)
        next_state["state"] = "awaiting_manual_confirmation"
        return next_state

    def complete_after_manual_confirmation(self, state: dict) -> dict:
        """Continue after the user confirms the external GoPay payment is done."""
        snap_token = str(state.get("snap_token") or "")
        cs_id = str(state.get("cs_id") or "")
        charge_ref = str(state.get("charge_ref") or "")
        if not charge_ref or not snap_token:
            raise GoPayError("payment flow state is missing charge_ref/snap_token")

        if self.midtrans_tokenization.lower() == "false":
            midtrans_status = self._midtrans_poll_status(snap_token)
            self._follow_midtrans_finish_redirect(state, midtrans_status)
        else:
            self._gopay_payment_validate(charge_ref)
            ch2_id, ch2_client = self._gopay_payment_confirm(charge_ref)
            pin_token2 = self._tokenize_pin(ch2_id, ch2_client, purpose="payment")
            self._gopay_payment_process(charge_ref, pin_token2)
            midtrans_status = self._midtrans_poll_status(snap_token)

        if cs_id:
            result = self._chatgpt_verify(cs_id)
            result.update({
                "snap_token": snap_token,
                "charge_ref": charge_ref,
                "midtrans_status": midtrans_status.get("transaction_status", ""),
                "deeplink_url": str(state.get("deeplink_url") or ""),
                "qr_code_url": str(state.get("qr_code_url") or ""),
                "finish_redirect_url": str(state.get("finish_redirect_url") or ""),
                "finish_200_redirect_url": str(state.get("finish_200_redirect_url") or ""),
            })
            return result
        return {
            "state": "succeeded",
            "snap_token": snap_token,
            "charge_ref": charge_ref,
            "midtrans_status": midtrans_status.get("transaction_status", ""),
            "deeplink_url": str(state.get("deeplink_url") or ""),
            "qr_code_url": str(state.get("qr_code_url") or ""),
            "finish_redirect_url": str(state.get("finish_redirect_url") or ""),
            "finish_200_redirect_url": str(state.get("finish_200_redirect_url") or ""),
        }

    def complete_after_otp(self, state: dict, otp: str) -> dict:
        """Resume a segmented GoPay flow after orchestrator supplies OTP."""
        next_state = self.complete_after_otp_until_manual_confirmation(state, otp)
        return self.complete_after_manual_confirmation(next_state)

    def _run_midtrans_and_gopay(
        self, snap_token: str, cs_id: str, stripe_pk: str = "",
    ) -> dict:
        state = self.start_linking_until_otp(snap_token, cs_id, stripe_pk)
        otp = self.otp_provider()
        return self.complete_after_otp(state, otp)


# ──────────────────────────── OTP providers ───────────────────────


def cli_otp_provider() -> str:
    """Read OTP from stdin (CLI mode)."""
    sys.stdout.write("\n[GoPay] Enter WhatsApp OTP: ")
    sys.stdout.flush()
    return sys.stdin.readline().strip()


def file_watch_otp_provider(watch_path: Path, timeout: float = 300.0) -> Callable[[], str]:
    """Build an OTP provider that polls a file for the OTP value.

    Used by external runners: emits 'GOPAY_OTP_REQUEST' marker on stdout, then
    blocks reading watch_path until an OTP appears.
    """

    def provider() -> str:
        # Signal to outer runner that OTP is needed
        print(f"GOPAY_OTP_REQUEST path={watch_path}", flush=True)
        deadline = time.time() + timeout
        while time.time() < deadline:
            if watch_path.exists():
                otp = watch_path.read_text(encoding="utf-8").strip()
                try:
                    watch_path.unlink()
                except FileNotFoundError:
                    pass
                if otp:
                    return otp
            time.sleep(0.5)
        raise OTPCancelled(f"OTP timeout after {timeout}s (file={watch_path})")

    return provider


_CHECKOUT_AMOUNT_KEYS = (
    "due",
    "amount_total",
    "amount_due",
    "total_amount",
    "amount_remaining",
    "total",
)
_CHECKOUT_AMOUNT_EXCLUDED_PATH_PARTS = {
    "amount_discount",
    "amount_subtotal",
    "amount_tax",
    "discount",
    "discounts",
    "display_items",
    "items",
    "line_items",
    "lines",
    "price",
    "prices",
    "subtotal",
    "tax",
    "taxes",
    "unit_amount",
}


def _bool_cfg(cfg: dict, key: str, default: bool = False) -> bool:
    value = cfg.get(key) if isinstance(cfg, dict) else None
    if value in (None, ""):
        return default
    if isinstance(value, bool):
        return value
    return str(value).strip().lower() in {"1", "true", "yes", "on"}


def _normalize_midtrans_tokenization(value: Any) -> str:
    if value in (None, ""):
        return "true"
    if isinstance(value, bool):
        return "true" if value else "false"
    return str(value).strip() or "true"


def _parse_stripe_amount(value: Any) -> Optional[int]:
    if isinstance(value, bool) or value in (None, ""):
        return None
    if isinstance(value, int):
        return value if value >= 0 else None
    text = str(value).strip()
    if re.fullmatch(r"\d+", text):
        return int(text)
    return None


def _path_has_amount_exclusion(path: tuple[str, ...]) -> bool:
    for part in path:
        if part.lower() in _CHECKOUT_AMOUNT_EXCLUDED_PATH_PARTS:
            return True
    return False


def _iter_checkout_amount_candidates(value: Any, path: tuple[str, ...] = ()) -> Any:
    if isinstance(value, dict):
        for key, child in value.items():
            key_text = str(key)
            child_path = path + (key_text,)
            normalized_key = key_text.lower()
            if (
                normalized_key in _CHECKOUT_AMOUNT_KEYS
                and not _path_has_amount_exclusion(child_path)
            ):
                amount = _parse_stripe_amount(child)
                if amount is not None:
                    yield ".".join(child_path), amount
            if isinstance(child, (dict, list)):
                yield from _iter_checkout_amount_candidates(child, child_path)
    elif isinstance(value, list):
        for idx, child in enumerate(value):
            if isinstance(child, (dict, list)):
                yield from _iter_checkout_amount_candidates(child, path + (str(idx),))


def _select_checkout_amount(init_data: dict) -> tuple[Optional[int], str]:
    candidates = list(_iter_checkout_amount_candidates(init_data))
    if not candidates:
        return None, "unknown"

    preferred_keys = (
        "due",
        "amount_total",
        "amount_due",
        "total_amount",
        "amount_remaining",
        "total",
    )
    preferred_contexts = ("total_summary", "checkout", "session", "invoice", "subscription")
    for key in preferred_keys:
        for source, amount in candidates:
            parts = tuple(source.lower().split("."))
            if parts[-1] != key:
                continue
            if len(parts) == 1 or any(ctx in parts for ctx in preferred_contexts):
                return amount, source
    return candidates[0][1], candidates[0][0]


def probe_plus_trial_checkout(
    chatgpt_session: Any,
    *,
    stripe_pk: str = DEFAULT_STRIPE_PK,
    runtime_cfg: Optional[dict] = None,
    checkout_cfg: Optional[dict] = None,
    proxy: Optional[str] = None,
    log: Callable[[str], None] = print,
) -> dict:
    """Create a promo trial checkout and read its real Stripe amount."""
    charger = GoPayCharger(
        chatgpt_session,
        {"country_code": "0", "phone_number": "0", "pin": "0"},
        otp_provider=lambda: (_ for _ in ()).throw(OTPCancelled("OTP not used by probe")),
        checkout_proxy=proxy,
        payment_proxy=proxy,
        runtime_cfg=runtime_cfg,
        checkout_cfg=checkout_cfg,
        log=log,
    )
    try:
        try:
            cs_id = charger._chatgpt_create_checkout()
        except Exception as exc:
            raise GoPayError(f"checkout create failed: {str(exc)[:500]}") from exc
        checkout_url = charger.checkout_url or f"https://checkout.stripe.com/c/pay/{cs_id}"

        try:
            init_data = charger._stripe_init(cs_id, stripe_pk)
        except Exception as exc:
            return {
                "checkout_session_id": cs_id,
                "checkout_url": checkout_url,
                "checked": False,
                "plus_trial_eligible": False,
                "amount": 0,
                "currency": "",
                "source": "stripe_init_error",
                "error_message": f"stripe init failed: {str(exc)[:500]}",
            }

        amount, source = _select_checkout_amount(init_data)
        checked = amount is not None
        currency = str(init_data.get("currency") or "").upper()
        return {
            "checkout_session_id": cs_id,
            "checkout_url": checkout_url,
            "checked": checked,
            "plus_trial_eligible": bool(checked and amount == 0),
            "amount": int(amount or 0),
            "currency": currency,
            "source": source,
            "error_message": "" if checked else "stripe init did not expose checkout amount",
        }
    finally:
        charger.close()


def _resolve_expected_amount(init_data: dict, runtime_cfg: Optional[dict]) -> tuple[str, str]:
    runtime_cfg = runtime_cfg if isinstance(runtime_cfg, dict) else {}
    override = runtime_cfg.get("expected_amount")
    if override not in (None, ""):
        amount = _parse_stripe_amount(override)
        if amount is None:
            raise GoPayError(f"invalid runtime.expected_amount: {override!r}")
        return str(amount), "runtime.expected_amount"

    amount, source = _select_checkout_amount(init_data)
    if amount is None:
        if _bool_cfg(runtime_cfg, "fail_on_unknown_expected_amount", False):
            raise GoPayError("stripe init did not expose checkout amount; refusing confirm")
        return "0", "fallback_zero_unknown"

    allow_nonzero = _bool_cfg(
        runtime_cfg,
        "allow_nonzero_expected_amount",
        _bool_cfg(runtime_cfg, "allow_paid_checkout", False),
    )
    if amount != 0 and not allow_nonzero:
        currency = str(init_data.get("currency") or "").upper() or "UNKNOWN"
        raise GoPayError(
            f"checkout amount is {amount} {currency} from {source}, not free-trial 0; "
            "refusing to confirm payment",
        )
    return str(amount), source


def _stripe_confirm_error_detail(
    text: str,
    *,
    expected_amount: str,
    expected_amount_source: str,
) -> str:
    try:
        payload = json.loads(text or "{}")
    except Exception:
        return ""
    error = payload.get("error") if isinstance(payload, dict) else None
    if not isinstance(error, dict):
        return ""
    if error.get("code") != "checkout_amount_mismatch":
        return ""
    message = str(error.get("message") or "checkout amount mismatch")
    return (
        "stripe confirm checkout_amount_mismatch: "
        f"sent expected_amount={expected_amount} from {expected_amount_source}; "
        f"{message}. This usually means the checkout/latest invoice amount changed "
        "after the session was created, or another checkout was created for the same account. "
        f"stripe_error={text[:400]}"
    )


# ──────────────────────────── chatgpt session ─────────────────────


def _build_chatgpt_session(auth_cfg: dict, proxy: Optional[str] = None) -> Any:
    """Build a chatgpt-authed session with chrome TLS fingerprint + OAI headers.

    /backend-api/payments/checkout requires: Cookie session-token, Bearer
    access_token, oai-device-id, x-openai-target-path/route, sentinel token.
    We supply everything except sentinel — caller refreshes via
    _ensure_sentinel before each protected call.
    """
    session_token = (auth_cfg.get("session_token") or "").strip()
    access_token = (auth_cfg.get("access_token") or "").strip()
    cookie_header = (auth_cfg.get("cookie_header") or "").strip()
    device_id = (auth_cfg.get("device_id") or "").strip() or str(uuid.uuid4())
    sentinel_token = (auth_cfg.get("openai_sentinel_token") or "").strip()
    user_agent = auth_cfg.get("user_agent") or (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
    )

    if not (session_token or cookie_header or access_token):
        raise GoPayError(
            "auth missing: need session_token, access_token, or cookie_header in config",
        )

    s = _new_session()
    if proxy:
        try:
            s.proxies = {"http": proxy, "https": proxy}
        except Exception:
            pass
    s.headers.update({
        "User-Agent": user_agent,
        "Accept": "*/*",
        "Accept-Language": "en-US,en;q=0.9",
        "Origin": "https://chatgpt.com",
        "Referer": "https://chatgpt.com/",
        "Content-Type": "application/json",
        "oai-device-id": device_id,
        "oai-language": "en-US",
        "sec-ch-ua": '"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"',
        "sec-ch-ua-mobile": "?0",
        "sec-ch-ua-platform": '"Windows"',
        "sec-fetch-dest": "empty",
        "sec-fetch-mode": "cors",
        "sec-fetch-site": "same-origin",
    })
    if access_token:
        s.headers["Authorization"] = f"Bearer {access_token}"
    if sentinel_token:
        s.headers["openai-sentinel-token"] = sentinel_token

    parts = []
    seen = set()

    def add_cookie_part(part: str) -> None:
        p = str(part or "").strip()
        if not p or "=" not in p:
            return
        n = _cookie_name(p)
        if n and n not in seen:
            seen.add(n)
            parts.append(p)

    for raw in (cookie_header or "").split(";"):
        add_cookie_part(raw)
    has_session_cookie = any(_session_cookie_name(name) for name in seen)
    if session_token and not has_session_cookie:
        for part in _session_cookie_parts(session_token):
            add_cookie_part(part)
    if device_id and "oai-did" not in seen:
        add_cookie_part(f"oai-did={device_id}")
    s.headers["Cookie"] = "; ".join(parts)
    try:
        if not (session_token or cookie_header):
            raise RuntimeError("skip session refresh without session cookie")
        r = s.get(
            "https://chatgpt.com/api/auth/session",
            headers={
                "User-Agent": user_agent,
                "Accept": "application/json",
                "Accept-Language": s.headers.get("Accept-Language", "en-US,en;q=0.9"),
                "Referer": "https://chatgpt.com/",
                "Cookie": s.headers["Cookie"],
            },
            timeout=DEFAULT_TIMEOUT,
        )
        if r.status_code == 200:
            refreshed_token = (r.json() or {}).get("accessToken") or ""
            if refreshed_token:
                s.headers["Authorization"] = f"Bearer {refreshed_token}"
    except Exception:
        pass
    # Cache device_id on session for subsequent header use
    s._oai_device_id = device_id  # type: ignore[attr-defined]
    return s


# ──────────────────────────── CLI entry ───────────────────────────


def _load_cfg(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def resolve_checkout_proxy(cfg: dict) -> Optional[str]:
    return (
        _clean_proxy(os.environ.get("GOPAY_CHECKOUT_PROXY_URL"))
        or _cfg_proxy(cfg, "proxies.checkout", "checkout_proxy")
        or None
    )


def resolve_payment_proxy(cfg: dict) -> Optional[str]:
    return (
        _clean_proxy(os.environ.get("GOPAY_PAYMENT_PROXY_URL"))
        or _cfg_proxy(cfg, "proxies.payment", "payment_proxy")
        or None
    )


def resolve_gopay_cfg(cfg: dict) -> dict:
    gopay_cfg = dict(cfg.get("gopay") or {})
    env_map = {
        "country_code": "GOPAY_COUNTRY_CODE",
        "phone_number": "GOPAY_PHONE_NUMBER",
        "pin": "GOPAY_PIN",
    }
    for key, env_key in env_map.items():
        value = (os.environ.get(env_key) or "").strip()
        if value:
            gopay_cfg[key] = value
    return gopay_cfg


def validate_gopay_cfg(gopay_cfg: dict) -> dict:
    missing = [
        key
        for key in ("country_code", "phone_number", "pin")
        if not str(gopay_cfg.get(key) or "").strip()
    ]
    if missing:
        raise GoPayError(
            "missing GoPay runtime config: "
            + ", ".join(f"GOPAY_{key.upper()}" for key in missing)
        )
    return gopay_cfg


def main():
    parser = argparse.ArgumentParser(
        description="ChatGPT Plus 订阅 via GoPay tokenization",
    )
    parser.add_argument("--config", required=True, help="GoPay config json")
    parser.add_argument("--otp-file", "--gopay-otp-file", dest="otp_file", default="",
                        help="poll this file for OTP (file deleted after read)")
    parser.add_argument("--otp-timeout", type=float, default=300.0,
                        help="seconds to wait for OTP file")
    parser.add_argument("--json-result", action="store_true",
                        help="Emit GOPAY_RESULT_JSON=... line on success")
    parser.add_argument("--session-token", default="",
                        help="Override ChatGPT __Secure-next-auth.session-token from config")
    parser.add_argument("--from-redirect-url", default="", metavar="URL",
                        help="半自动模式：跳过 chatgpt+stripe 前段，直接从 pm-redirects.stripe.com URL 接管 Midtrans+GoPay")
    parser.add_argument("--cs-id", default="", help="可选：cs_live_xxx，verify 阶段用")
    args = parser.parse_args()

    cfg = _load_cfg(args.config)
    try:
        gopay_cfg = validate_gopay_cfg(resolve_gopay_cfg(cfg))
    except GoPayError as e:
        print(f"[error] {e}", file=sys.stderr)
        sys.exit(2)

    auth_cfg = (cfg.get("fresh_checkout") or {}).get("auth") or {}
    session_token = args.session_token.strip()
    if session_token:
        auth_cfg = dict(auth_cfg)
        auth_cfg["session_token"] = session_token
        auth_cfg.pop("cookie_header", None)
        auth_cfg.pop("access_token", None)
    checkout_proxy = resolve_checkout_proxy(cfg)
    payment_proxy = resolve_payment_proxy(cfg)
    try:
        cs_session = _build_chatgpt_session(auth_cfg, proxy=checkout_proxy)
    except GoPayError as e:
        print(f"[error] {e}", file=sys.stderr)
        sys.exit(2)

    stripe_pk = (
        (cfg.get("stripe") or {}).get("publishable_key")
        or auth_cfg.get("stripe_pk")
        or DEFAULT_STRIPE_PK
    )

    billing = cfg.get("billing") or {}
    if not billing:
        cards = cfg.get("cards") or []
        if cards and isinstance(cards[0], dict):
            card0 = cards[0]
            billing = dict(card0.get("address") or {})
            for key in ("name", "email"):
                if card0.get(key):
                    billing[key] = card0[key]

    if args.otp_file:
        provider = file_watch_otp_provider(Path(args.otp_file), timeout=args.otp_timeout)
    else:
        provider = cli_otp_provider

    charger = GoPayCharger(
        cs_session, gopay_cfg,
        otp_provider=provider,
        checkout_proxy=checkout_proxy,
        payment_proxy=payment_proxy,
        runtime_cfg=cfg.get("runtime"),
        checkout_cfg=dict((cfg.get("fresh_checkout") or {}).get("plan") or {}),
        browser_challenge_cfg=dict(cfg.get("browser_challenge") or {}),
        pre_solve_passive_captcha=bool(cfg.get("pre_solve_passive_captcha", False)),
    )
    try:
        if args.from_redirect_url:
            print(f"[gopay] semi-auto mode: starting from {args.from_redirect_url[:80]}...")
            result = charger.run_from_redirect(args.from_redirect_url, cs_id=args.cs_id)
        else:
            result = charger.run(stripe_pk=stripe_pk, billing=billing)
    except GoPayError as e:
        print(f"[gopay] FAILED: {e}", file=sys.stderr)
        if args.json_result:
            print(f"GOPAY_RESULT_JSON={json.dumps({'state':'failed','error':str(e)})}")
        sys.exit(1)

    print(f"[gopay] result: {result}")
    if args.json_result:
        print(f"GOPAY_RESULT_JSON={json.dumps(result, ensure_ascii=False)}")


if __name__ == "__main__":
    main()
