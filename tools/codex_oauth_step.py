#!/usr/bin/env python3
"""Manual single-step Camoufox controller for Codex OAuth.

Use this when the automatic pilot needs visual inspection. It launches Camoufox,
prints/saves a screenshot and DOM control summary after every command, and lets
an operator issue explicit fill/click/continue/otp/phone commands.
"""

from __future__ import annotations

import argparse
import getpass
import json
import os
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import codex_oauth_local as base

CONTROL_SELECTOR = 'input, button, select, textarea, a, [role="combobox"], [role="option"]'


def parse_args() -> argparse.Namespace:
    root = Path(__file__).resolve().parents[1]
    parser = argparse.ArgumentParser(description="Manual single-step Codex OAuth Camoufox flow")
    parser.add_argument("--email", required=True)
    parser.add_argument("--password-env", default="CODEX_OAUTH_PASSWORD")
    parser.add_argument("--proxy-server", default=os.environ.get("CODEX_OAUTH_PROXY_SERVER"))
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--artifact-dir", type=Path, default=Path("/tmp/codex-oauth-step"))
    parser.add_argument("--run-log", type=Path, default=root / "docs" / "codex-oauth-camoufox-local.md")
    parser.add_argument("--headless", action="store_true")
    parser.add_argument("--port", type=int, default=base.DEFAULT_PORT)
    parser.add_argument("--width", type=int, default=1365)
    parser.add_argument("--height", type=int, default=768)
    return parser.parse_args()


def snapshot(page, artifact_dir: Path, label: str, logger: base.StepLog) -> list[dict]:
    artifact_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    safe_label = "".join(ch if ch.isalnum() or ch in "-_" else "_" for ch in label)[:80]
    prefix = artifact_dir / f"{stamp}-{safe_label}"
    screenshot = prefix.with_suffix(".png")
    summary_path = prefix.with_suffix(".json")
    page.screenshot(path=str(screenshot), full_page=True)
    summary = page.evaluate(
        """selector => ({
            url: location.href,
            title: document.title,
            bodyText: document.body ? document.body.innerText.slice(0, 4000) : "",
            controls: Array.from(document.querySelectorAll(selector)).slice(0, 120).map((el, index) => ({
                index,
                tag: el.tagName,
                type: el.getAttribute('type'),
                role: el.getAttribute('role'),
                name: el.getAttribute('name'),
                id: el.id,
                autocomplete: el.getAttribute('autocomplete'),
                placeholder: el.getAttribute('placeholder'),
                ariaLabel: el.getAttribute('aria-label'),
                text: (el.innerText || '').slice(0, 160),
                value: (el.type === 'password' ? '<password>' : (el.value || '')).slice(0, 160),
                visible: !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length),
            })),
        })""",
        CONTROL_SELECTOR,
    )
    summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    logger.record("PASS", f"step_snapshot_{label}", f"url={base.sanitize_url(summary['url'])}, screenshot={screenshot}, summary={summary_path}")
    print(f"\nURL: {base.sanitize_url(summary['url'])}")
    print(f"TITLE: {summary['title']}")
    print(f"SCREENSHOT: {screenshot}")
    print("CONTROLS:")
    for control in summary["controls"]:
        if not control.get("visible"):
            continue
        desc = control_desc(control)
        print(f"  [{control['index']}] {desc}")
    print()
    return summary["controls"]


def control_desc(control: dict) -> str:
    fields = []
    for key in ("tag", "type", "role", "name", "id", "autocomplete", "placeholder", "ariaLabel", "text", "value"):
        value = control.get(key)
        if value:
            fields.append(f"{key}={value!r}")
    return " ".join(fields)


def locator_by_index(page, index: int):
    return page.locator(CONTROL_SELECTOR).nth(index)


def fill_likely(page, selectors: tuple[str, ...], value: str, label: str, logger: base.StepLog) -> None:
    locator, selector = base.visible_locator(page, selectors, timeout_ms=1500)
    if locator is None:
        raise RuntimeError(f"{label} input not found")
    locator.fill(value, timeout=5000)
    logger.record("PASS", f"fill_{label}", f"selector=`{selector}`")


def click_likely(page, selectors: tuple[str, ...], label: str, logger: base.StepLog) -> None:
    if not base.click_first(page, selectors, label, logger):
        raise RuntimeError(f"{label} button/link not found")


def maybe_finish(capture: base.CallbackCapture, args: argparse.Namespace, pkce: base.PkceCodes, logger: base.StepLog) -> bool:
    if not capture.event.is_set():
        return False
    if capture.error:
        raise RuntimeError(f"OAuth callback error: {capture.error}: {capture.error_description or ''}")
    logger.record("PASS", "callback_seen", base.sanitize_url(capture.raw_url or capture.redirect_uri))
    tokens = base.exchange_code_for_tokens(capture.code or "", capture.redirect_uri, pkce, args.proxy_server)
    base.write_auth_json(args.output.expanduser().resolve(), tokens)
    base.validate_auth_json(args.output.expanduser().resolve())
    logger.record("PASS", "auth_json_written", str(args.output.expanduser().resolve()))
    print(f"DONE auth_json={args.output.expanduser().resolve()}")
    return True


def print_help() -> None:
    print(
        "Commands:\n"
        "  snap [label]              save screenshot + DOM summary\n"
        "  email                     fill configured email\n"
        "  password                  fill password from env/prompt\n"
        "  otp <code>                fill OTP code\n"
        "  phone <raw>               select country if possible, fill local phone\n"
        "  continue                  click Continue/Next/submit\n"
        "  click <index>             click visible control index\n"
        "  fill <index> <value>      fill control index\n"
        "  press <key>               press key, e.g. Enter or Escape\n"
        "  back                      browser history back\n"
        "  goto <url>                navigate current page\n"
        "  wait [seconds]            wait and resnapshot\n"
        "  finish                    exchange callback code and write auth.json\n"
        "  help                      show commands\n"
        "  quit                      stop\n"
    )


def main() -> int:
    args = parse_args()
    logger = base.StepLog(args.run_log)
    server, capture = base.bind_callback_server(args.port)
    password_cache: str | None = os.environ.get(args.password_env)
    try:
        pkce = base.generate_pkce()
        state = base.b64url_random(32)
        authorize_url = base.build_authorize_url(capture.redirect_uri, pkce, state)
        logger.record("PASS", "callback_server_started", f"redirect_uri={capture.redirect_uri}")
        logger.record("PASS", "oauth_url_generated", base.sanitize_url(authorize_url))

        from camoufox.sync_api import Camoufox

        launch_options = {"headless": args.headless}
        proxy = base.parse_proxy_option(args.proxy_server)
        if proxy:
            launch_options["proxy"] = proxy
            logger.record("PASS", "proxy_configured", base.sanitize_url(proxy["server"]))

        with Camoufox(**launch_options) as browser:
            page = browser.new_page(viewport={"width": args.width, "height": args.height})
            page.goto(authorize_url, wait_until="domcontentloaded", timeout=60000)
            base.wait_for_settle(page)
            controls = snapshot(page, args.artifact_dir, "start", logger)
            print_help()
            step = 0
            while True:
                if maybe_finish(capture, args, pkce, logger):
                    return 0
                command = input("codex-oauth-step> ").strip()
                if not command:
                    controls = snapshot(page, args.artifact_dir, f"step-{step}", logger)
                    step += 1
                    continue
                parts = command.split()
                verb = parts[0].lower()
                try:
                    if verb in {"quit", "exit"}:
                        return 1
                    if verb == "help":
                        print_help()
                    elif verb == "snap":
                        controls = snapshot(page, args.artifact_dir, parts[1] if len(parts) > 1 else f"step-{step}", logger)
                        step += 1
                    elif verb == "email":
                        fill_likely(page, base.EMAIL_SELECTORS, args.email, "email", logger)
                    elif verb == "password":
                        if not password_cache:
                            password_cache = getpass.getpass(f"Password for {base.mask_email(args.email)}: ")
                        fill_likely(page, base.PASSWORD_SELECTORS, password_cache, "password", logger)
                    elif verb == "otp":
                        code = command.partition(" ")[2].strip()
                        if not code:
                            raise RuntimeError("otp requires a code")
                        if not base.fill_code(page, code):
                            raise RuntimeError("OTP input not found")
                        logger.record("PASS", "otp_filled", f"digits={len(code)}")
                    elif verb == "phone":
                        raw = command.partition(" ")[2].strip()
                        if not raw:
                            raise RuntimeError("phone requires a value")
                        country_code, local = base.normalize_phone(raw)
                        selected = base.select_country_code(page, country_code, logger)
                        value = local if selected or not country_code else f"+{country_code}{local}"
                        fill_likely(page, base.PHONE_SELECTORS + base.PHONE_FALLBACK_SELECTORS, value, "phone", logger)
                    elif verb == "continue":
                        click_likely(page, base.CONTINUE_BUTTON_SELECTORS, "continue", logger)
                    elif verb == "click":
                        locator_by_index(page, int(parts[1])).click(timeout=5000)
                        logger.record("PASS", "click_index", parts[1])
                        base.wait_for_settle(page)
                    elif verb == "fill":
                        if len(parts) < 3:
                            raise RuntimeError("fill requires index and value")
                        value = command.split(maxsplit=2)[2]
                        locator_by_index(page, int(parts[1])).fill(value, timeout=5000)
                        logger.record("PASS", "fill_index", parts[1])
                    elif verb == "press":
                        key = parts[1] if len(parts) > 1 else "Enter"
                        page.keyboard.press(key)
                        logger.record("PASS", "press", key)
                    elif verb == "back":
                        page.go_back(wait_until="domcontentloaded", timeout=30000)
                        logger.record("PASS", "go_back", base.sanitize_url(page.url))
                    elif verb == "goto":
                        url = command.partition(" ")[2].strip()
                        if not url:
                            raise RuntimeError("goto requires a URL")
                        page.goto(url, wait_until="domcontentloaded", timeout=60000)
                        logger.record("PASS", "goto", base.sanitize_url(page.url))
                    elif verb == "wait":
                        seconds = float(parts[1]) if len(parts) > 1 else 2.0
                        time.sleep(seconds)
                    elif verb == "finish":
                        if not capture.event.wait(5):
                            raise RuntimeError("callback not captured yet")
                        if maybe_finish(capture, args, pkce, logger):
                            return 0
                    else:
                        print(f"unknown command: {verb}")
                        print_help()
                    base.wait_for_settle(page)
                    controls = snapshot(page, args.artifact_dir, f"step-{step}-{verb}", logger)
                    step += 1
                except Exception as exc:
                    logger.record("FAIL", f"command_{verb}", str(exc))
                    controls = snapshot(page, args.artifact_dir, f"step-{step}-{verb}-failed", logger)
                    step += 1
        return 1
    except Exception as exc:
        logger.record("FAIL", "codex_oauth_step", str(exc))
        return 1
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    sys.exit(main())
