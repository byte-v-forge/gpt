import re
from typing import Iterable, Mapping


SESSION_COOKIE_BASES = (
    "__Secure-next-auth.session-token",
    "next-auth.session-token",
    "__Secure-authjs.session-token",
    "authjs.session-token",
)


def is_session_cookie_name(name: str) -> bool:
    clean = str(name or "").strip()
    for base in SESSION_COOKIE_BASES:
        if clean == base:
            return True
        prefix = base + "."
        if clean.startswith(prefix) and clean[len(prefix):].isdigit():
            return True
    return False


def session_cookie_sort_key(name: str) -> tuple[int, int, str]:
    clean = str(name or "").strip()
    for base_order, base in enumerate(SESSION_COOKIE_BASES):
        if clean == base:
            return (base_order, -1, clean)
        prefix = base + "."
        if clean.startswith(prefix):
            suffix = clean[len(prefix):]
            if suffix.isdigit():
                return (base_order, int(suffix), clean)
    return (99, 0, clean)


def extract_session_token(cookies: Iterable[Mapping[str, str]]) -> str:
    parts: list[tuple[str, str]] = []
    for cookie in cookies:
        name = str(cookie.get("name", "")).strip()
        value = str(cookie.get("value", "")).strip()
        if value and is_session_cookie_name(name):
            parts.append((name, value))
    if not parts:
        return ""

    parts.sort(key=lambda item: session_cookie_sort_key(item[0]))
    first_name = parts[0][0]
    if not re.search(r"\.\d+$", first_name):
        return parts[0][1]
    return "".join(value for _, value in parts)
