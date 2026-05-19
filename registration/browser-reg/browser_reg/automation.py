import logging
import os
import re
import shutil
import tempfile
import time
from typing import Any, Callable

from browser_reg.sensitive import sanitize_text

logger = logging.getLogger(__name__)


class BrowserRegistrationCancelled(RuntimeError):
    pass


def _interruptible_sleep(seconds: float, check_cancel: Callable[[], None]) -> None:
    deadline = time.time() + max(0.0, seconds)
    while True:
        check_cancel()
        remaining = deadline - time.time()
        if remaining <= 0:
            return
        time.sleep(min(0.25, remaining))


def _env_bool(name: str, default: bool = False) -> bool:
    value = os.environ.get(name, "").strip().lower()
    if not value:
        return default
    return value in {"1", "true", "yes", "on"}


def _env_int(name: str, default: int) -> int:
    value = os.environ.get(name, "").strip()
    if not value:
        return default
    try:
        return int(value)
    except ValueError:
        return default


def _env_str(name: str, default: str) -> str:
    value = os.environ.get(name, "").strip()
    return value or default


def browser_locale() -> str:
    return _env_str("BROWSER_REG_LOCALE", "en-US")


def browser_languages() -> list[str]:
    raw = _env_str("BROWSER_REG_LANGUAGES", f"{browser_locale()},en")
    languages: list[str] = []
    for item in re.split(r"[\s,]+", raw):
        item = item.strip()
        if item and item not in languages:
            languages.append(item)
    return languages or ["en-US", "en"]


def browser_accept_language() -> str:
    languages = browser_languages()
    if len(languages) == 1:
        return languages[0]
    return ", ".join(
        lang if index == 0 else f"{lang};q={max(0.1, 1.0 - index * 0.1):.1f}"
        for index, lang in enumerate(languages)
    )


def browser_timezone() -> str:
    return os.environ.get("BROWSER_REG_TIMEZONE", "").strip()


def browser_window_size() -> tuple[int, int]:
    width = max(800, _env_int("BROWSER_REG_WINDOW_WIDTH", 1365))
    height = max(600, _env_int("BROWSER_REG_WINDOW_HEIGHT", 768))
    return width, height


def browser_firefox_user_prefs() -> dict[str, Any]:
    return {
        "intl.accept_languages": browser_accept_language(),
        "intl.locale.requested": browser_locale(),
        "javascript.use_us_english_locale": True,
    }


def browser_process_env() -> dict[str, str]:
    env = dict(os.environ)
    env.update({
        "LANG": "en_US.UTF-8",
        "LC_ALL": "en_US.UTF-8",
        "LANGUAGE": "en_US:en",
    })
    return env


def apply_browser_language_overrides(ctx) -> None:
    languages = browser_languages()
    locale = languages[0]
    try:
        ctx.set_extra_http_headers({"Accept-Language": browser_accept_language()})
    except Exception as e:
        logger.info("[browser-reg] set Accept-Language failed: %s", sanitize_text(e))

    script = f"""
(() => {{
  const language = {locale!r};
  const languages = {languages!r};
  const define = (object, property, value) => {{
    try {{
      Object.defineProperty(object, property, {{
        get: () => value,
        configurable: true,
      }});
    }} catch (_) {{}}
  }};
  define(Navigator.prototype, 'language', language);
  define(Navigator.prototype, 'languages', languages);
}})();
"""
    try:
        ctx.add_init_script(script)
    except Exception as e:
        logger.info("[browser-reg] language init script failed: %s", sanitize_text(e))


def is_playwright_target_closed_error(error: Exception) -> bool:
    text = str(error).lower()
    return (
        "target page, context or browser has been closed" in text
        or "page has been closed" in text
        or "browser has been closed" in text
        or "context has been closed" in text
    )


def cleanup_stale_browser_profiles(prefix: str, max_age_seconds: float = 4 * 3600) -> int:
    now = time.time()
    removed = 0
    tmp_root = tempfile.gettempdir()
    try:
        names = os.listdir(tmp_root)
    except OSError:
        return 0
    for name in names:
        if not name.startswith(prefix):
            continue
        path = os.path.join(tmp_root, name)
        try:
            if now - os.path.getmtime(path) < max_age_seconds:
                continue
            shutil.rmtree(path, ignore_errors=True)
            removed += 1
        except OSError:
            continue
    return removed
