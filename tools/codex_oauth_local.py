#!/usr/bin/env python3
"""Local Camoufox proof flow for Codex OAuth auth.json generation.

This is intentionally a local, operator-driven pilot: Camoufox, the OAuth
localhost callback server, token exchange, and auth.json materialization all run
in one process on the same host. OTP / add-phone branches pause for manual input
so the OAuth and auth.json path can be verified before wiring mailbox/SMS
services.
"""

from __future__ import annotations

import argparse
import base64
import dataclasses
import getpass
import hashlib
import html
import json
import os
import re
import secrets
import socket
import sys
import tempfile
import threading
import time
from datetime import datetime, timezone
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Iterable
from urllib import error as url_error
from urllib import parse, request

ISSUER = "https://auth.openai.com"
CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann"
ORIGINATOR = "codex_cli_rs"
DEFAULT_PORT = 1455
FALLBACK_PORT = 1457
SCOPE = "openid profile email offline_access api.connectors.read api.connectors.invoke"
SENSITIVE_QUERY_KEYS = {
    "access_token",
    "api_key",
    "client_secret",
    "code",
    "code_challenge",
    "code_verifier",
    "id_token",
    "key",
    "refresh_token",
    "requested_token",
    "state",
    "subject_token",
    "token",
}
EMAIL_SELECTORS = (
    'input#email[name="email"]',
    'input[name="email"][type="email"]',
    'input[type="email"]',
    'input[autocomplete="email"]',
)
PASSWORD_SELECTORS = (
    'input[name="current-password"][type="password"]',
    'input[type="password"]',
    'input[autocomplete="current-password"]',
)
OTP_SELECTORS = (
    'input[autocomplete="one-time-code"]',
    'input[inputmode="numeric"]',
    'input[name="code"]',
)
PHONE_SELECTORS = (
    'input[autocomplete="tel"]',
    'input[name*="phone" i]',
    'input[id*="phone" i]',
)
PHONE_FALLBACK_SELECTORS = ('input[type="tel"]',)
CONTINUE_BUTTON_SELECTORS = (
    'button:has-text("Continue")',
    'button:has-text("Next")',
    'button[type="submit"]',
    'input[type="submit"]',
)
CONSENT_BUTTON_SELECTORS = (
    'button:has-text("Authorize")',
    'button:has-text("Allow")',
    'button:has-text("Continue")',
)
CONTINUE_WITH_PASSWORD_SELECTORS = (
    'a:has-text("Continue with password")',
    'button:has-text("Continue with password")',
)
COUNTRY_NAMES = {
    "54": "Argentina",
    "66": "Thailand",
}


@dataclasses.dataclass(frozen=True)
class PkceCodes:
    verifier: str
    challenge: str


@dataclasses.dataclass
class CallbackCapture:
    port: int
    redirect_uri: str
    event: threading.Event = dataclasses.field(default_factory=threading.Event)
    raw_url: str | None = None
    code: str | None = None
    state: str | None = None
    error: str | None = None
    error_description: str | None = None


class StepLog:
    def __init__(self, path: Path | None) -> None:
        self.path = path
        if path is not None:
            path.parent.mkdir(parents=True, exist_ok=True)
            if not path.exists():
                path.write_text(
                    "# Codex OAuth Local Camoufox Verification\n\n"
                    "## Run log\n\n",
                    encoding="utf-8",
                )

    def record(self, status: str, step: str, detail: str = "") -> None:
        safe_detail = detail.strip()
        prefix = f"[{status}] {step}"
        print(f"{prefix}: {safe_detail}" if safe_detail else prefix, flush=True)
        if self.path is None:
            return
        now = datetime.now(timezone.utc).isoformat(timespec="seconds")
        line = f"- `{now}` `{status}` **{step}**"
        if safe_detail:
            line += f": {safe_detail}"
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(line + "\n")


def b64url_random(byte_count: int) -> str:
    return base64.urlsafe_b64encode(secrets.token_bytes(byte_count)).rstrip(b"=").decode()


def generate_pkce() -> PkceCodes:
    verifier = b64url_random(64)
    digest = hashlib.sha256(verifier.encode()).digest()
    challenge = base64.urlsafe_b64encode(digest).rstrip(b"=").decode()
    return PkceCodes(verifier=verifier, challenge=challenge)


def build_authorize_url(redirect_uri: str, pkce: PkceCodes, state: str) -> str:
    params = {
        "response_type": "code",
        "client_id": CLIENT_ID,
        "redirect_uri": redirect_uri,
        "scope": SCOPE,
        "code_challenge": pkce.challenge,
        "code_challenge_method": "S256",
        "id_token_add_organizations": "true",
        "codex_cli_simplified_flow": "true",
        "state": state,
        "originator": ORIGINATOR,
    }
    return f"{ISSUER}/oauth/authorize?{parse.urlencode(params)}"


def sanitize_url(raw_url: str) -> str:
    try:
        parsed = parse.urlsplit(raw_url)
    except ValueError:
        return "<invalid-url>"
    netloc = parsed.hostname or ""
    if parsed.port:
        netloc = f"{netloc}:{parsed.port}"
    query = []
    for key, value in parse.parse_qsl(parsed.query, keep_blank_values=True):
        if key.lower() in SENSITIVE_QUERY_KEYS:
            query.append((key, "<redacted>"))
        else:
            query.append((key, value))
    safe = parsed._replace(netloc=netloc, query=parse.urlencode(query), fragment="")
    return parse.urlunsplit(safe)


def parse_proxy_option(proxy_server: str | None) -> dict[str, str] | None:
    if not proxy_server:
        return None
    parsed = parse.urlsplit(proxy_server)
    if not parsed.scheme or not parsed.hostname:
        raise RuntimeError("proxy server must include scheme and host, for example http://127.0.0.1:10810")
    if parsed.path not in ("", "/") or parsed.query or parsed.fragment:
        raise RuntimeError("proxy server must not include path, query, or fragment")
    server_netloc = parsed.hostname
    if parsed.port:
        server_netloc = f"{server_netloc}:{parsed.port}"
    proxy: dict[str, str] = {
        "server": parse.urlunsplit((parsed.scheme, server_netloc, "", "", "")),
        "bypass": "localhost,127.0.0.1",
    }
    if parsed.username:
        proxy["username"] = parse.unquote(parsed.username)
    if parsed.password:
        proxy["password"] = parse.unquote(parsed.password)
    return proxy


def mask_email(email: str) -> str:
    local, sep, domain = email.partition("@")
    if not sep:
        return "<redacted-email>"
    if len(local) <= 2:
        masked_local = local[0] + "*" if local else "*"
    else:
        masked_local = f"{local[0]}***{local[-1]}"
    return f"{masked_local}@{domain}"


def mask_phone(value: str) -> str:
    digits = re.sub(r"\D+", "", value)
    if len(digits) <= 4:
        return "<redacted-phone>"
    return f"***{digits[-4:]}"


def normalize_phone(raw_phone: str) -> tuple[str | None, str]:
    digits = re.sub(r"\D+", "", raw_phone)
    if digits.startswith("00"):
        digits = digits[2:]
    for code in sorted(COUNTRY_NAMES, key=len, reverse=True):
        if digits.startswith(code) and len(digits) > len(code) + 5:
            local = digits[len(code):]
            if code == "66" and local.startswith("0"):
                local = local[1:]
            return code, local
    return None, digits


def decode_jwt_payload(jwt: str) -> dict:
    parts = jwt.split(".")
    if len(parts) != 3 or not parts[1]:
        return {}
    payload = parts[1] + "=" * (-len(parts[1]) % 4)
    try:
        return json.loads(base64.urlsafe_b64decode(payload.encode()))
    except (ValueError, json.JSONDecodeError):
        return {}


def extract_auth_claims(id_token: str) -> tuple[str | None, str | None]:
    claims = decode_jwt_payload(id_token)
    profile = claims.get("https://api.openai.com/profile") or {}
    auth = claims.get("https://api.openai.com/auth") or {}
    email = claims.get("email") or profile.get("email")
    account_id = auth.get("chatgpt_account_id")
    return email, account_id


def write_auth_json(output: Path, tokens: dict) -> None:
    email, account_id = extract_auth_claims(tokens["id_token"])
    token_payload = {
        "id_token": tokens["id_token"],
        "access_token": tokens["access_token"],
        "refresh_token": tokens["refresh_token"],
    }
    if account_id:
        token_payload["account_id"] = account_id
    auth_json = {
        "auth_mode": "chatgpt",
        "OPENAI_API_KEY": None,
        "tokens": token_payload,
        "last_refresh": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=f".{output.name}.", suffix=".tmp", dir=output.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(auth_json, handle, ensure_ascii=False, indent=2)
            handle.write("\n")
        os.chmod(tmp_name, 0o600)
        os.replace(tmp_name, output)
        os.chmod(output, 0o600)
    finally:
        if os.path.exists(tmp_name):
            os.unlink(tmp_name)
    _ = email


def exchange_code_for_tokens(code: str, redirect_uri: str, pkce: PkceCodes, proxy_server: str | None) -> dict:
    data = parse.urlencode(
        {
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": redirect_uri,
            "client_id": CLIENT_ID,
            "code_verifier": pkce.verifier,
        }
    ).encode()
    req = request.Request(
        f"{ISSUER}/oauth/token",
        data=data,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        method="POST",
    )
    opener = request.build_opener()
    if proxy_server:
        if parse.urlsplit(proxy_server).scheme.startswith("socks"):
            raise RuntimeError("token exchange proxy only supports http/https proxy URLs in this local pilot")
        opener = request.build_opener(request.ProxyHandler({"http": proxy_server, "https": proxy_server}))
    try:
        with opener.open(req, timeout=30) as resp:
            payload = json.loads(resp.read().decode())
    except url_error.HTTPError as exc:
        detail = _token_error_detail(exc)
        raise RuntimeError(f"token endpoint returned status {exc.code}: {detail}") from exc
    except url_error.URLError as exc:
        raise RuntimeError(f"token exchange transport failed: {exc.reason}") from exc
    for key in ("id_token", "access_token", "refresh_token"):
        if not payload.get(key):
            raise RuntimeError(f"token endpoint response is missing {key}")
    return payload


def _token_error_detail(exc: url_error.HTTPError) -> str:
    try:
        body = exc.read(4096).decode(errors="replace")
        parsed = json.loads(body)
    except Exception:
        return "unknown error"
    if isinstance(parsed, dict):
        err = parsed.get("error")
        if isinstance(err, dict):
            return str(err.get("message") or err.get("code") or "unknown error")
        return str(parsed.get("error_description") or err or "unknown error")
    return "unknown error"


def bind_callback_server(preferred_port: int) -> tuple[ThreadingHTTPServer, CallbackCapture]:
    last_error: OSError | None = None
    for port in (preferred_port, FALLBACK_PORT):
        capture = CallbackCapture(port=port, redirect_uri=f"http://localhost:{port}/auth/callback")
        handler = make_callback_handler(capture)
        try:
            server = ThreadingHTTPServer(("127.0.0.1", port), handler)
        except OSError as exc:
            last_error = exc
            continue
        thread = threading.Thread(target=server.serve_forever, name="codex-oauth-callback", daemon=True)
        thread.start()
        return server, capture
    raise RuntimeError(f"cannot bind callback server: {last_error}")


def make_callback_handler(capture: CallbackCapture) -> type[BaseHTTPRequestHandler]:
    class CallbackHandler(BaseHTTPRequestHandler):
        def log_message(self, fmt: str, *args: object) -> None:  # noqa: A003
            return

        def do_GET(self) -> None:  # noqa: N802
            parsed = parse.urlsplit(self.path)
            if parsed.path == "/cancel":
                self.send_response(HTTPStatus.OK)
                self.end_headers()
                return
            if parsed.path != "/auth/callback":
                self.send_response(HTTPStatus.NOT_FOUND)
                self.end_headers()
                return
            query = parse.parse_qs(parsed.query)
            capture.raw_url = f"http://localhost:{capture.port}{self.path}"
            capture.code = first_query_value(query, "code")
            capture.state = first_query_value(query, "state")
            capture.error = first_query_value(query, "error")
            capture.error_description = first_query_value(query, "error_description")
            capture.event.set()
            self.send_response(HTTPStatus.OK)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.end_headers()
            title = "Codex OAuth callback captured"
            message = "You can return to the terminal."
            if capture.error:
                title = "Codex OAuth failed"
                message = html.escape(capture.error_description or capture.error)
            body = f"<html><body><h1>{title}</h1><p>{message}</p></body></html>"
            self.wfile.write(body.encode())

    return CallbackHandler


def first_query_value(query: dict[str, list[str]], key: str) -> str | None:
    values = query.get(key) or []
    return values[0] if values else None


def port_is_available(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.settimeout(0.2)
        return sock.connect_ex(("127.0.0.1", port)) != 0


def visible_locator(page, selectors: Iterable[str], timeout_ms: int = 500):
    for selector in selectors:
        locator = page.locator(selector).first
        try:
            locator.wait_for(state="visible", timeout=timeout_ms)
            return locator, selector
        except Exception:
            continue
    return None, None


def click_first(page, selectors: Iterable[str], label: str, logger: StepLog) -> bool:
    locator, selector = visible_locator(page, selectors)
    if locator is None:
        return False
    locator.click(timeout=5_000)
    logger.record("PASS", f"click_{label}", f"selector=`{selector}`")
    wait_for_settle(page)
    return True


def wait_for_settle(page, timeout_ms: int = 8_000) -> None:
    try:
        page.wait_for_load_state("domcontentloaded", timeout=timeout_ms)
    except Exception:
        pass
    try:
        page.wait_for_timeout(700)
    except Exception:
        time.sleep(0.7)


def fill_code(page, code: str) -> bool:
    locator, _selector = visible_locator(page, OTP_SELECTORS)
    if locator is None:
        return False
    try:
        locator.fill(code, timeout=3_000)
    except Exception:
        locator.click(timeout=3_000)
        page.keyboard.type(code)
    return True


def capture_page_artifacts(page, artifact_dir: Path | None, name: str, logger: StepLog) -> None:
    if artifact_dir is None:
        return
    artifact_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    prefix = artifact_dir / f"{stamp}-{name}"
    screenshot_path = prefix.with_suffix(".png")
    summary_path = prefix.with_suffix(".json")
    try:
        page.screenshot(path=str(screenshot_path), full_page=True)
    except Exception as exc:
        logger.record("WAIT", f"{name}_screenshot_failed", str(exc))
        screenshot_path = None
    try:
        summary = page.evaluate(
            """() => ({
                url: location.href,
                title: document.title,
                bodyText: document.body ? document.body.innerText.slice(0, 4000) : "",
                controls: Array.from(document.querySelectorAll('input, button, select, [role="combobox"], [role="option"]')).slice(0, 80).map((el) => ({
                    tag: el.tagName,
                    type: el.getAttribute('type'),
                    role: el.getAttribute('role'),
                    name: el.getAttribute('name'),
                    id: el.id,
                    autocomplete: el.getAttribute('autocomplete'),
                    placeholder: el.getAttribute('placeholder'),
                    ariaLabel: el.getAttribute('aria-label'),
                    text: (el.innerText || el.value || '').slice(0, 200),
                })),
            })"""
        )
        summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    except Exception as exc:
        logger.record("WAIT", f"{name}_summary_failed", str(exc))
        summary_path = None
    parts = []
    if screenshot_path:
        parts.append(f"screenshot={screenshot_path}")
    if summary_path:
        parts.append(f"summary={summary_path}")
    if parts:
        logger.record("PASS", f"{name}_artifacts", ", ".join(parts))


def select_country_code(page, country_code: str | None, logger: StepLog) -> bool:
    if not country_code:
        return False
    country_name = COUNTRY_NAMES.get(country_code, country_code)
    target = re.compile(rf"({re.escape(country_name)}|\\+?{re.escape(country_code)}\\b)", re.IGNORECASE)

    selects = page.locator("select")
    try:
        count = selects.count()
    except Exception:
        count = 0
    for i in range(count):
        select = selects.nth(i)
        try:
            if not select.is_visible(timeout=500):
                continue
            options = select.locator("option").evaluate_all(
                """options => options.map((o, index) => ({
                    index,
                    value: o.value,
                    text: o.innerText || o.textContent || "",
                }))"""
            )
            for option in options:
                text = f"{option.get('text', '')} {option.get('value', '')}"
                if target.search(text):
                    select.select_option(index=option["index"])
                    logger.record("PASS", "country_selected", f"select option={country_name} +{country_code}")
                    wait_for_settle(page, timeout_ms=2_000)
                    return True
        except Exception:
            continue

    combo_selectors = (
        '[role="combobox"]',
        'button[aria-haspopup="listbox"]',
        'button:has-text("+")',
        'button:has-text("United States")',
        'button:has-text("Country")',
    )
    for selector in combo_selectors:
        combo, combo_selector = visible_locator(page, (selector,), timeout_ms=700)
        if combo is None:
            continue
        try:
            combo.click(timeout=3_000)
            wait_for_settle(page, timeout_ms=1_000)
            search, search_selector = visible_locator(
                page,
                (
                    'input[role="searchbox"]',
                    'input[placeholder*="Search" i]',
                    'input[type="search"]',
                    'input[type="text"]',
                ),
                timeout_ms=800,
            )
            if search is not None:
                search.fill(country_name, timeout=2_000)
                logger.record("PASS", "country_search_filled", f"selector=`{search_selector}`, country={country_name}")
                wait_for_settle(page, timeout_ms=1_000)
            try:
                page.get_by_role("option", name=target).first.click(timeout=3_000)
            except Exception:
                page.get_by_text(target).first.click(timeout=3_000)
            logger.record("PASS", "country_selected", f"selector=`{combo_selector}`, country={country_name} +{country_code}")
            wait_for_settle(page, timeout_ms=2_000)
            return True
        except Exception as exc:
            logger.record("WAIT", "country_select_attempt_failed", f"selector=`{combo_selector}`, error={exc}")
            try:
                page.keyboard.press("Escape")
            except Exception:
                pass
    logger.record("WAIT", "country_select_not_found", f"country={country_name} +{country_code}")
    return False


def page_has_phone_prompt(page) -> bool:
    try:
        body_text = page.locator("body").inner_text(timeout=1_000)
    except Exception:
        return False
    return bool(re.search(r"\b(phone|mobile|sms)\b", body_text, re.IGNORECASE))


def run_browser_flow(args, authorize_url: str, capture: CallbackCapture, logger: StepLog) -> None:
    try:
        from camoufox.sync_api import Camoufox
    except ImportError as exc:
        raise RuntimeError("camoufox is not installed; install `camoufox[geoip]` for this local pilot") from exc

    password = os.environ.get(args.password_env)
    if not password:
        password = getpass.getpass(f"Password for {mask_email(args.email)} ({args.password_env} not set): ")
    logger.record("PASS", "credentials_loaded", f"email={mask_email(args.email)}, password_source={args.password_env}")

    launch_options = {"headless": args.headless}
    proxy = parse_proxy_option(args.proxy_server)
    if proxy:
        launch_options["proxy"] = proxy
        logger.record("PASS", "proxy_configured", sanitize_url(proxy["server"]))

    with Camoufox(**launch_options) as browser:
        page = browser.new_page(viewport={"width": args.width, "height": args.height})
        logger.record("PASS", "camoufox_started", f"headless={args.headless}, viewport={args.width}x{args.height}")
        page.goto(authorize_url, wait_until="domcontentloaded", timeout=args.page_timeout_ms)
        logger.record("PASS", "oauth_url_opened", sanitize_url(page.url))
        wait_for_settle(page)
        drive_login_pages(page, args, password, capture, logger)


def drive_login_pages(page, args, password: str, capture: CallbackCapture, logger: StepLog) -> None:
    email_submitted = False
    password_submitted = False
    phone_attempted = False
    idle_count = 0
    for _ in range(args.max_steps):
        if capture.event.is_set():
            logger.record("PASS", "callback_observed", sanitize_url(capture.raw_url or page.url))
            return
        current_url = page.url
        if current_url.startswith(capture.redirect_uri):
            capture.raw_url = current_url
            capture.event.set()
            logger.record("PASS", "callback_url_seen_in_browser", sanitize_url(current_url))
            return

        if click_first(page, CONTINUE_WITH_PASSWORD_SELECTORS, "continue_with_password", logger):
            idle_count = 0
            continue

        email_locator, email_selector = visible_locator(page, EMAIL_SELECTORS)
        if email_locator is not None and not email_submitted:
            email_locator.fill(args.email, timeout=5_000)
            logger.record("PASS", "email_filled", f"selector=`{email_selector}`, email={mask_email(args.email)}")
            click_first(page, CONTINUE_BUTTON_SELECTORS, "email_continue", logger)
            email_submitted = True
            idle_count = 0
            continue

        password_locator, password_selector = visible_locator(page, PASSWORD_SELECTORS)
        if password_locator is not None and not password_submitted:
            password_locator.fill(password, timeout=5_000)
            logger.record("PASS", "password_filled", f"selector=`{password_selector}`")
            click_first(page, CONTINUE_BUTTON_SELECTORS, "password_continue", logger)
            password_submitted = True
            idle_count = 0
            continue

        otp_locator, otp_selector = visible_locator(page, OTP_SELECTORS)
        if otp_locator is not None and not phone_attempted:
            code = input(f"OTP detected ({otp_selector}). Enter code, or press Enter after manual browser completion: ").strip()
            if code:
                fill_code(page, code)
                logger.record("PASS", "otp_filled", f"selector=`{otp_selector}`, digits={len(code)}")
                click_first(page, CONTINUE_BUTTON_SELECTORS, "otp_continue", logger)
            else:
                logger.record("WAIT", "otp_manual_continue", "operator completed this branch in browser")
            idle_count = 0
            continue

        phone_locator, phone_selector = visible_locator(page, PHONE_SELECTORS)
        if phone_locator is None and page_has_phone_prompt(page):
            phone_locator, phone_selector = visible_locator(page, PHONE_FALLBACK_SELECTORS)
        if phone_locator is not None and not phone_attempted:
            capture_page_artifacts(page, args.artifact_dir, "phone_detected", logger)
            phone_attempted = True
            if args.phone:
                phone = args.phone.strip()
            else:
                phone = input(
                    f"Add-phone detected ({phone_selector}). Paste phone to autofill, "
                    "or press Enter after manual browser entry: "
                ).strip()
            if phone:
                country_code, local_phone = normalize_phone(phone)
                country_selected = select_country_code(page, country_code, logger)
                fill_value = local_phone if country_selected or not country_code else f"+{country_code}{local_phone}"
                phone_locator, phone_selector = visible_locator(page, PHONE_SELECTORS + PHONE_FALLBACK_SELECTORS)
                if phone_locator is None:
                    raise RuntimeError("phone input disappeared before fill")
                phone_locator.fill(fill_value, timeout=5_000)
                logger.record("PASS", "phone_filled", f"selector=`{phone_selector}`, phone={mask_phone(fill_value)}")
                capture_page_artifacts(page, args.artifact_dir, "phone_filled", logger)
                click_first(page, CONTINUE_BUTTON_SELECTORS, "phone_continue", logger)
            else:
                logger.record("WAIT", "phone_manual_continue", "operator entered phone in browser")
            sms = input("Enter SMS code if page asks for it, or press Enter after manual completion: ").strip()
            if sms:
                fill_code(page, sms)
                logger.record("PASS", "sms_filled", f"digits={len(sms)}")
                click_first(page, CONTINUE_BUTTON_SELECTORS, "sms_continue", logger)
            idle_count = 0
            continue

        if click_first(page, CONSENT_BUTTON_SELECTORS, "oauth_consent", logger):
            idle_count = 0
            continue

        idle_count += 1
        logger.record("WAIT", "page_wait", sanitize_url(current_url))
        if idle_count >= args.manual_after_idle and not args.headless:
            input("Unknown/stable page. Complete visible browser step if needed, then press Enter to continue: ")
            idle_count = 0
        wait_for_settle(page, timeout_ms=3_000)

    raise RuntimeError(f"OAuth browser flow did not finish after {args.max_steps} steps; url={sanitize_url(page.url)}")


def validate_auth_json(path: Path) -> None:
    payload = json.loads(path.read_text(encoding="utf-8"))
    tokens = payload.get("tokens") or {}
    missing = [
        key
        for key in ("id_token", "access_token", "refresh_token")
        if not tokens.get(key)
    ]
    if payload.get("auth_mode") != "chatgpt":
        missing.append("auth_mode=chatgpt")
    if not payload.get("last_refresh"):
        missing.append("last_refresh")
    if missing:
        raise RuntimeError(f"auth.json validation failed; missing {', '.join(missing)}")


def parse_args() -> argparse.Namespace:
    root = Path(__file__).resolve().parents[1]
    parser = argparse.ArgumentParser(description="Run local Camoufox Codex OAuth and write auth.json")
    parser.add_argument("--email", required=True, help="OpenAI account email. Only a masked form is logged.")
    parser.add_argument("--password-env", default="CODEX_OAUTH_PASSWORD", help="Env var containing the account password.")
    parser.add_argument("--output", required=True, type=Path, help="Destination auth.json path. File is written 0600.")
    parser.add_argument("--run-log", type=Path, default=root / "docs" / "codex-oauth-camoufox-local.md")
    parser.add_argument("--headless", action="store_true", help="Run Camoufox headless. Headed mode is recommended for the pilot.")
    parser.add_argument("--proxy-server", default=os.environ.get("CODEX_OAUTH_PROXY_SERVER"), help="Proxy URL for browser and token exchange, for example http://127.0.0.1:10810.")
    parser.add_argument("--phone", default=os.environ.get("CODEX_OAUTH_PHONE"), help="Optional phone to fill on add-phone page; Thai +66 input is split into country and local number.")
    parser.add_argument("--artifact-dir", type=Path, default=Path("/tmp/codex-oauth-artifacts"), help="Directory for Camoufox screenshots and DOM summaries.")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT, help="Preferred local callback port.")
    parser.add_argument("--width", type=int, default=1365)
    parser.add_argument("--height", type=int, default=768)
    parser.add_argument("--page-timeout-ms", type=int, default=60_000)
    parser.add_argument("--callback-timeout-seconds", type=int, default=300)
    parser.add_argument("--max-steps", type=int, default=120)
    parser.add_argument("--manual-after-idle", type=int, default=5)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    logger = StepLog(args.run_log)
    if not port_is_available(args.port):
        logger.record("WAIT", "preferred_port_busy", f"port={args.port}, fallback={FALLBACK_PORT}")
    server, capture = bind_callback_server(args.port)
    try:
        logger.record("PASS", "callback_server_started", f"redirect_uri={capture.redirect_uri}")
        pkce = generate_pkce()
        state = b64url_random(32)
        authorize_url = build_authorize_url(capture.redirect_uri, pkce, state)
        logger.record("PASS", "oauth_url_generated", sanitize_url(authorize_url))
        run_browser_flow(args, authorize_url, capture, logger)
        if not capture.event.wait(args.callback_timeout_seconds):
            raise RuntimeError("OAuth callback was not received before timeout")
        if capture.error:
            raise RuntimeError(f"OAuth callback error: {capture.error}: {capture.error_description or ''}")
        if capture.state != state:
            raise RuntimeError("OAuth callback state mismatch; auth.json was not written")
        if not capture.code:
            raise RuntimeError("OAuth callback did not include an authorization code")
        logger.record("PASS", "callback_validated", sanitize_url(capture.raw_url or capture.redirect_uri))
        tokens = exchange_code_for_tokens(capture.code, capture.redirect_uri, pkce, args.proxy_server)
        logger.record("PASS", "token_exchange_succeeded", "id/access/refresh tokens present")
        write_auth_json(args.output.expanduser().resolve(), tokens)
        validate_auth_json(args.output.expanduser().resolve())
        logger.record("PASS", "auth_json_written", str(args.output.expanduser().resolve()))
        return 0
    except Exception as exc:
        logger.record("FAIL", "codex_oauth_local", str(exc))
        return 1
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    sys.exit(main())
