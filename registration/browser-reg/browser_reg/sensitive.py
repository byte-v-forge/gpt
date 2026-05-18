"""Helpers for keeping secrets out of logs and debug output."""

from __future__ import annotations

import re
from urllib.parse import urlsplit


_EMAIL_RE = re.compile(r"(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b")
_URL_CREDENTIALS_RE = re.compile(r"([a-z][a-z0-9+.-]*://)([^/\s:@]+):([^/\s@]+)@", re.I)
_BEARER_RE = re.compile(r"(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{12,}")
_IPV4_RE = re.compile(r"\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b")
_SENSITIVE_ASSIGNMENT_RE = re.compile(
    r"(?i)\b(session[_-]?token|access[_-]?token|csrf[_-]?token|cookie[_-]?header|password)"
    r"\s*[:=]\s*['\"]?[^'\"\s,}]{6,}"
)

def redact_email(value: str | None) -> str:
    if not value:
        return ""
    text = str(value).strip()
    if "@" not in text:
        return "<redacted>"
    local, domain = text.rsplit("@", 1)
    if not local:
        return f"***@{domain}"
    if len(local) <= 2:
        masked = local[0] + "***"
    else:
        masked = f"{local[:2]}***"
    return f"{masked}@{domain}"


def sanitize_text(value: object) -> str:
    text = str(value)
    text = _URL_CREDENTIALS_RE.sub(r"\1***:***@", text)
    text = _EMAIL_RE.sub(lambda match: redact_email(match.group(0)), text)
    text = _IPV4_RE.sub("<redacted-ip>", text)
    text = _BEARER_RE.sub("Bearer <redacted>", text)
    text = _SENSITIVE_ASSIGNMENT_RE.sub(lambda match: f"{match.group(1)}=<redacted>", text)
    return text


def sanitize_url_for_log(value: str | None, max_len: int = 120) -> str:
    text = str(value or "")
    try:
        parsed = urlsplit(text)
    except ValueError:
        return sanitize_text(text)[:max_len]
    if parsed.scheme and parsed.netloc:
        safe = f"{parsed.scheme}://{parsed.netloc}{parsed.path}"
    else:
        safe = text.split("?", 1)[0].split("#", 1)[0]
    return sanitize_text(safe)[:max_len]
