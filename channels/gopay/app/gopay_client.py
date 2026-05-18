"""
GoPay/Gojek mobile API request wrapper.

The 2026-05-13 captures in gopay-capture show the GoPay 2.7.0 request
shape: compact JSON bodies, stable device/session headers, D1/X-M1/X-E1/X-E2
security headers, X-E3 as the exact body MD5, and AdjTs=host:D.
"""

import base64
import hashlib
import hmac
import json
import os
import random
import time
import uuid
from typing import Optional
from urllib.parse import urlparse

import requests


HMAC_KEY = os.environ.get("GOPAY_HMAC_KEY", "")
DEFAULT_EMPTY_MD5 = "d41d8cd98f00b204e9800998ecf8427e"
DEFAULT_X_E2 = os.environ.get("GOPAY_X_E2", "")
COMPACT_JSON_SEPARATORS = (",", ":")
GOPAY_CUSTOMER_SLIM_GET_PATHS = {
    "/v1/users/profile",
    "/v1/payment-options/balances",
    "/v1/payment-options/profiles",
    "/v1/user/wallet-card/balance",
}
GOPAY_CUSTOMER_APP_HEADER_PATHS = {
    "/v1/users/profile",
    "/v1/qris/payments",
    "/v2/customer/payment-options/checkout/list",
    "/v1/customer/payment-options/settings/last-used",
    "/v1/promotions/evaluate",
    "/api/v1/festival-envelopes/claim",
    "/api/v1/users/deactivate",
    "/api/v1/users/deactivate/check",
    "/api/v1/users/pin/challenges",
    "/api/v1/users/pin/tokens",
    "/api/v1/users/pin/tokens/nb",
    "/api/v1/users/pins/allowed",
    "/api/v2/users/pins/setup/tokens",
    "/cvs/v1/methods",
    "/cvs/v1/initiate",
    "/cvs/v1/verify",
}
GOJEK_ACTIVITY_PATHS = {
    "/v5/customers",
    "/v2/otp/retry",
    "/v5/customers/verificationUpdateProfile",
    "/gojek/v2/customer",
}
GOJEK_APP_HEADER_PATHS = {
    "/courier/v1/token",
    "/v7/customers/signup",
}
GOPAY_CUSTOMER_LINKED_APP_PATH = "/v1/linkedapps"
GOPAY_CUSTOMER_LINK_PREFIX = "/v1/links/"

def _random_d1() -> str:
    return ":".join(f"{b:02X}" for b in os.urandom(32))


def _random_widevine_id() -> str:
    return base64.b64encode(hashlib.sha256(os.urandom(32)).digest()).decode()


def _random_appsflyer_id() -> str:
    return (
        f"{int(time.time() * 1000)}-"
        f"{random.randint(1000000000000000000, 9999999999999999999)}"
    )


def _random_wifi_mac() -> str:
    return "02:" + ":".join(f"{b:02x}" for b in os.urandom(5))


def _random_wifi_ssid() -> str:
    prefix = _random_brand_word()
    return f"{prefix}_{os.urandom(6).hex()}"


def _random_letters(length: int, alphabet: str = "ABCDEFGHIJKLMNOPQRSTUVWXYZ") -> str:
    return "".join(random.choice(alphabet) for _ in range(length))


def _random_brand_word() -> str:
    consonants = "bcdfghjklmnpqrstvwxyz"
    vowels = "aeiou"
    syllables = []
    for _ in range(random.randint(2, 4)):
        syllables.append(random.choice(consonants) + random.choice(vowels))
    if random.random() < 0.35:
        syllables.append(random.choice(("n", "r", "s", "x")))
    return "".join(syllables).capitalize()


def _random_phone_make() -> str:
    return _random_brand_word()


def _random_phone_model(make: str) -> str:
    prefix = "".join(ch for ch in make.upper() if ch.isalpha())[:2]
    if len(prefix) < 2:
        prefix = _random_letters(2)
    family = random.choice(("A", "C", "M", "N", "P", "R", "S", "V", "X", "Z"))
    number = random.randint(1000, 9999)
    suffix = _random_letters(random.randint(0, 2))
    separator = random.choice(("-", " "))
    return f"{make}, {prefix}{family}{separator}{number}{suffix}"


def _random_screen() -> str:
    width = random.randrange(720, 1448, 16)
    aspect = random.uniform(1.95, 2.28)
    height = int(width * aspect)
    height = min(3200, max(width + 640, (height // 8) * 8))
    screen = f"{width}x{height}"
    return "1088x2160" if screen == "1080x2148" else screen


def _random_android_version() -> str:
    return str(random.randint(10, 14))


def _random_device_shape() -> dict:
    make = _random_phone_make()
    return {
        "make": make,
        "model": _random_phone_model(make),
        "screen": _random_screen(),
        "android_version": _random_android_version(),
    }


def _env_flag(name: str, default: bool = False) -> bool:
    value = os.environ.get(name, "")
    if value == "":
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _use_env_identity(use_env_identity: Optional[bool]) -> bool:
    if use_env_identity is not None:
        return bool(use_env_identity)
    return _env_flag("GOPAY_STATIC_DEVICE_IDENTITY")


def _identity_value(key: str, fallback, use_env_identity: bool) -> str:
    if use_env_identity:
        value = os.environ.get(key, "")
        if value:
            return value
    return fallback() if callable(fallback) else str(fallback)


def generate_device_fingerprint(randomize_model: Optional[bool] = None, use_env_identity: Optional[bool] = None) -> dict:
    """生成并持久化到 state 的设备/header 种子。"""
    use_env_identity = _use_env_identity(use_env_identity)
    shape = _random_device_shape()
    make, model = shape["make"], shape["model"]
    android_version = _identity_value("GOPAY_ANDROID_VERSION", shape["android_version"], use_env_identity)
    app_version = os.environ.get("GOPAY_APP_VERSION", "2.7.0")
    app_id = os.environ.get("GOPAY_APP_ID", "com.gojek.gopay")
    app_build = os.environ.get("GOPAY_APP_BUILD", "2070")
    d1 = os.environ.get("GOPAY_D1", "") if use_env_identity else ""
    if not d1 and os.environ.get("GOPAY_RANDOM_D1", "1") == "1":
        d1 = _random_d1()

    return {
        "x-apptype": "GOPAY",
        "x-appversion": app_version,
        "x-appid": app_id,
        "x-platform": "Android",
        "x-uniqueid": _identity_value("GOPAY_UNIQUE_ID", lambda: os.urandom(8).hex(), use_env_identity),
        "x-phonemake": make,
        "x-phonemodel": model,
        "x-deviceos": _identity_value("GOPAY_DEVICE_OS", f"Android, {android_version}", use_env_identity),
        "x-user-type": "customer",
        "x-session-id": _identity_value("GOPAY_SESSION_ID", lambda: str(uuid.uuid4()), use_env_identity),
        "transaction-id": _identity_value("GOPAY_TRANSACTION_ID", lambda: str(uuid.uuid4()), use_env_identity),
        "user-agent": _identity_value(
            "GOPAY_USER_AGENT",
            f"GoPay/{app_version} ({app_id}; build:{app_build}; Android, {android_version})",
            use_env_identity,
        ),
        "d1": d1,
        "x-e2": os.environ.get("GOPAY_X_E2", DEFAULT_X_E2),
        "adjts": os.environ.get("GOPAY_ADJTS", "host:D"),
        "m1_appsflyer_id": _identity_value("GOPAY_APPSFLYER_ID", _random_appsflyer_id, use_env_identity),
        "m1_widevine_id": _identity_value("GOPAY_WIDEVINE_ID", _random_widevine_id, use_env_identity),
        "m1_screen": _identity_value("GOPAY_SCREEN", shape["screen"], use_env_identity),
        "m1_wifi_mac": _identity_value("GOPAY_WIFI_MAC", _random_wifi_mac, use_env_identity),
        "m1_wifi_ssid": _random_wifi_ssid(),
        "m1_connection_id": str(random.randint(100000, 999999)),
        "m1_signature": _identity_value("GOPAY_M1_SIGNATURE", lambda: os.urandom(16).hex(), use_env_identity),
        "m1_device_uuid": str(uuid.uuid4()),
        "user-uuid": _identity_value("GOPAY_USER_UUID", "", use_env_identity),
        "x-devicetoken": _identity_value("GOPAY_DEVICE_TOKEN", "", use_env_identity),
        "x-location": _identity_value("GOPAY_LOCATION", lambda: f"{round(-6.2 + random.uniform(-0.05, 0.05), 7)},{round(106.8 + random.uniform(-0.05, 0.05), 7)}", use_env_identity),
        "x-location-accuracy": _identity_value("GOPAY_LOCATION_ACCURACY", lambda: f"0.0{random.randint(10, 99)}999999552965164", use_env_identity),
        "gojek-country-code": os.environ.get("GOPAY_GOJEK_COUNTRY_CODE", "ID"),
    }


def generate_random_device_fingerprint(randomize_model: Optional[bool] = None) -> dict:
    if randomize_model is None:
        randomize_model = True
    return generate_device_fingerprint(randomize_model=randomize_model, use_env_identity=False)


# 默认设备（可被覆盖）
DEVICE = generate_device_fingerprint()

GOTO_CLIENT_ID = os.environ.get("GOTO_CLIENT_ID", "gopay:consumer:app")
GOTO_CLIENT_SECRET = os.environ.get("GOTO_CLIENT_SECRET", "")
GOTO_SSO_CLIENT_ID = os.environ.get("GOTO_SSO_CLIENT_ID", "gojek:consumer:app")
GOTO_SSO_CLIENT_SECRET = os.environ.get("GOTO_SSO_CLIENT_SECRET", "")


def _proxy_map(proxy: Optional[str]) -> Optional[dict]:
    if not proxy:
        return None
    value = proxy.strip()
    if value.startswith("socks5://"):
        value = "socks5h://" + value[len("socks5://"):]
    return {"http": value, "https": value}


def _compact_json(body: Optional[dict]) -> str:
    if body is None:
        return ""
    return json.dumps(body, ensure_ascii=False, separators=COMPACT_JSON_SEPARATORS)


def _device_get(device: dict, key: str, default: str = "") -> str:
    return str(device.get(key) or device.get(key.lower()) or device.get(key.upper()) or default)


def _ensure_device_defaults(device: dict) -> dict:
    use_env_identity = _use_env_identity(None)
    shape = _random_device_shape()
    android_version = _identity_value("GOPAY_ANDROID_VERSION", shape["android_version"], use_env_identity)
    device.setdefault("x-apptype", "GOPAY")
    device.setdefault("x-appversion", os.environ.get("GOPAY_APP_VERSION", "2.7.0"))
    device.setdefault("x-appid", os.environ.get("GOPAY_APP_ID", "com.gojek.gopay"))
    device.setdefault("x-platform", "Android")
    device.setdefault("x-uniqueid", _identity_value("GOPAY_UNIQUE_ID", lambda: os.urandom(8).hex(), use_env_identity))
    device.setdefault("x-phonemake", shape["make"])
    device.setdefault("x-phonemodel", shape["model"])
    device.setdefault("x-deviceos", _identity_value("GOPAY_DEVICE_OS", f"Android, {android_version}", use_env_identity))
    device.setdefault("x-user-type", "customer")
    device.setdefault("x-session-id", _identity_value("GOPAY_SESSION_ID", lambda: str(uuid.uuid4()), use_env_identity))
    device.setdefault("transaction-id", _identity_value("GOPAY_TRANSACTION_ID", lambda: str(uuid.uuid4()), use_env_identity))
    device.setdefault(
        "user-agent",
        _identity_value(
            "GOPAY_USER_AGENT",
            f"GoPay/{device['x-appversion']} ({device['x-appid']}; build:{os.environ.get('GOPAY_APP_BUILD', '2070')}; Android, {android_version})",
            use_env_identity,
        ),
    )
    if not _device_get(device, "d1"):
        device["d1"] = _identity_value("GOPAY_D1", _random_d1, use_env_identity)
    device.setdefault("x-e2", os.environ.get("GOPAY_X_E2", DEFAULT_X_E2))
    device.setdefault("adjts", os.environ.get("GOPAY_ADJTS", "host:D"))
    device.setdefault("m1_appsflyer_id", _identity_value("GOPAY_APPSFLYER_ID", _random_appsflyer_id, use_env_identity))
    device.setdefault("m1_widevine_id", _identity_value("GOPAY_WIDEVINE_ID", _random_widevine_id, use_env_identity))
    device.setdefault("m1_screen", _identity_value("GOPAY_SCREEN", shape["screen"], use_env_identity))
    device.setdefault("m1_wifi_mac", _identity_value("GOPAY_WIFI_MAC", _random_wifi_mac, use_env_identity))
    device.setdefault("m1_wifi_ssid", _random_wifi_ssid())
    device.setdefault("m1_connection_id", str(random.randint(100000, 999999)))
    device.setdefault("m1_signature", _identity_value("GOPAY_M1_SIGNATURE", lambda: os.urandom(16).hex(), use_env_identity))
    device.setdefault("m1_device_uuid", str(uuid.uuid4()))
    device.setdefault("user-uuid", _identity_value("GOPAY_USER_UUID", "", use_env_identity))
    device.setdefault("x-devicetoken", _identity_value("GOPAY_DEVICE_TOKEN", "", use_env_identity))
    device.setdefault("x-location", _identity_value("GOPAY_LOCATION", lambda: f"{round(-6.2 + random.uniform(-0.05, 0.05), 7)},{round(106.8 + random.uniform(-0.05, 0.05), 7)}", use_env_identity))
    device.setdefault("x-location-accuracy", _identity_value("GOPAY_LOCATION_ACCURACY", lambda: f"0.0{random.randint(10, 99)}999999552965164", use_env_identity))
    device.setdefault("gojek-country-code", os.environ.get("GOPAY_GOJEK_COUNTRY_CODE", "ID"))
    return device


def ensure_device_fingerprint(device: Optional[dict] = None) -> dict:
    return _ensure_device_defaults(device if isinstance(device, dict) else {})


def _current_x_m1(device: dict) -> str:
    return ",".join(
        [
            f"3:{_device_get(device, 'm1_appsflyer_id', _random_appsflyer_id())}",
            f"4:{_device_get(device, 'm1_connection_id', '5939')}",
            f"5:{_device_get(device, 'x-phonemake')}|3200|2",
            f"6:{_device_get(device, 'm1_wifi_mac', '02:00:00:00:00:00')}",
            f"7:{_device_get(device, 'm1_wifi_ssid', '<unknown ssid>')}",
            f"8:{_device_get(device, 'm1_screen', '1080x2148')}",
            "9:passive,network,fused,gps",
            "10:1",
            f"11:{_device_get(device, 'm1_widevine_id', _random_widevine_id())}",
            f"15:{_device_get(device, 'm1_signature')}",
            f"16:{_device_get(device, 'm1_device_uuid')}",
        ]
    )


def _security_host(url: str) -> str:
    return urlparse(url).netloc.lower()


def _security_path(url: str) -> str:
    return urlparse(url).path


def _is_gopay_customer_link_path(path: str) -> bool:
    return path == GOPAY_CUSTOMER_LINKED_APP_PATH or path.startswith(GOPAY_CUSTOMER_LINK_PREFIX)


def _is_gopay_customer_app_header_path(path: str) -> bool:
    if path in GOPAY_CUSTOMER_APP_HEADER_PATHS:
        return True
    if path == "/v1/festivals" or path.startswith("/v1/festivals/"):
        return True
    if path.startswith("/customers/v1/payments/"):
        return True
    if path.startswith("/v3/payments/") and path.endswith("/capture"):
        return True
    if path.startswith("/api/v2/challenges/") and (path.endswith("/pin-page") or path.endswith("/pin-page/nb")):
        return True
    return False


def generate_xe1(
    method: str,
    url: str,
    body: str,
    token: str,
    device: dict = None,
    x_m1: str = "",
) -> tuple:
    """Generate X-E1 header. Returns (xe1, body_md5)."""
    if device is None:
        device = DEVICE
    _ensure_device_defaults(device)
    body_md5 = hashlib.md5(body.encode()).hexdigest() if body else DEFAULT_EMPTY_MD5
    override = os.environ.get("GOPAY_X_E1", "")
    if override:
        return override, body_md5
    if not HMAC_KEY:
        raise RuntimeError("GOPAY_HMAC_KEY is required to generate X-E1")

    field1 = os.urandom(32).hex() + "0" * 64 + os.urandom(16).hex()
    ts = str(int(time.time() * 1000))
    path = url.removeprefix("https://").removeprefix("http://")
    jwt = token.removeprefix("Bearer ")
    x_m1 = x_m1 or _current_x_m1(device)

    parts = [
        _device_get(device, "x-apptype", "GOPAY"),
        f"{_device_get(device, 'x-phonemodel')}:{jwt}",
        f"{_device_get(device, 'x-uniqueid')}:",
        f"{body_md5}:{path}",
        f"{method}:{ts}",
        f"{_device_get(device, 'x-deviceos')}:{_device_get(device, 'x-appversion')}",
        f"{x_m1}:{_device_get(device, 'x-appid')}",
        f"{field1}:{_device_get(device, 'x-phonemake')}",
        _device_get(device, "x-platform", "Android"),
    ]
    msg = ";".join(parts)
    first64 = hmac.new(HMAC_KEY.encode(), msg.encode(), hashlib.sha256).hexdigest()
    xe1 = f"{first64}:{field1}:{os.environ.get('GOPAY_X_E1_MARKER', 'D')}:{ts}"
    return xe1, body_md5


def _header_value(headers: dict, name: str, default=None):
    wanted = name.lower()
    for key, value in headers.items():
        if str(key).lower() == wanted:
            return str(value or "")
    return default


class GopayClient:
    def __init__(self, token: str, proxy: Optional[str] = None, device: Optional[dict] = None):
        self.token = token
        self.proxy = proxy
        self.device = ensure_device_fingerprint(device)
        self.session = requests.Session()
        self.session.headers.clear()

    def new_fingerprint(self):
        """切换到新的随机设备指纹"""
        self.device = generate_random_device_fingerprint(randomize_model=True)
        return self.device

    def _headers(self, method: str, url: str, body_str: str, extra_headers: Optional[dict]) -> dict:
        host = _security_host(url)
        path = _security_path(url)
        x_m1 = _current_x_m1(self.device)
        body_md5 = hashlib.md5(body_str.encode()).hexdigest() if body_str else DEFAULT_EMPTY_MD5
        has_body = body_str != ""
        headers = {
            "X-AppVersion": _device_get(self.device, "x-appversion", "2.7.0"),
            "X-AppId": _device_get(self.device, "x-appid", "com.gojek.gopay"),
            "X-AppType": _device_get(self.device, "x-apptype", "GOPAY"),
            "Accept": "application/json",
            "User-Agent": _device_get(self.device, "user-agent"),
            "D1": _device_get(self.device, "d1"),
            "X-Session-ID": _device_get(self.device, "x-session-id"),
            "X-Platform": _device_get(self.device, "x-platform", "Android"),
            "X-UniqueId": _device_get(self.device, "x-uniqueid"),
            "X-User-Type": _device_get(self.device, "x-user-type", "customer"),
            "X-DeviceOS": _device_get(self.device, "x-deviceos", "Android, 11"),
            "X-PhoneMake": _device_get(self.device, "x-phonemake", "Google"),
            "X-PushTokenType": "FCM",
            "X-PhoneModel": _device_get(self.device, "x-phonemodel", "Google, sdk_gphone_arm64"),
            "Accept-Language": "en-ID",
            "X-User-Locale": "en_ID",
            "X-M1": x_m1,
            "X-E2": _device_get(self.device, "x-e2", DEFAULT_X_E2),
            "X-E3": body_md5,
            "AdjTs": _device_get(self.device, "adjts", "host:D"),
        }
        if has_body:
            headers["Content-Type"] = "application/json"

        def app_headers() -> dict:
            out = {
                "Accept-Encoding": "gzip",
                "Gojek-Service-Area": "1",
                "Country-Code": _device_get(self.device, "gojek-country-code", "ID"),
                "X-AppVersion": _device_get(self.device, "x-appversion", "2.7.0"),
                "X-M1": x_m1,
                "Gojek-Country-Code": _device_get(self.device, "gojek-country-code", "ID"),
                "X-Request-ID": str(uuid.uuid1()),
                "X-UniqueId": _device_get(self.device, "x-uniqueid"),
                "X-PhoneMake": _device_get(self.device, "x-phonemake", "Google"),
                "X-Help-Version": _device_get(self.device, "x-appversion", "2.7.0"),
                "X-Location": _device_get(self.device, "x-location"),
                "X-Location-Accuracy": _device_get(self.device, "x-location-accuracy"),
                "X-DeviceOS": _device_get(self.device, "x-deviceos", "Android, 11"),
                "X-User-Type": _device_get(self.device, "x-user-type", "customer"),
                "User-Agent": _device_get(self.device, "user-agent"),
                "X-AppId": _device_get(self.device, "x-appid", "com.gojek.gopay"),
                "Gojek-Timezone": os.environ.get("GOPAY_TIMEZONE", "Asia/Jakarta"),
                "X-AuthSDK-Version": os.environ.get("GOPAY_AUTHSDK_VERSION", "1.0.0"),
                "X-AppType": _device_get(self.device, "x-apptype", "GOPAY"),
                "X-User-Locale": os.environ.get("GOPAY_USER_LOCALE", "en_ID"),
                "X-DeviceToken": _device_get(self.device, "x-devicetoken"),
                "X-E2": _device_get(self.device, "x-e2", DEFAULT_X_E2),
                "X-CVSDK-Version": os.environ.get("GOPAY_CVSDK_VERSION", "1.0.0"),
                "Accept-Language": os.environ.get("GOPAY_ACCEPT_LANGUAGE", "en-ID"),
                "Transaction-ID": _device_get(self.device, "transaction-id"),
                "X-PhoneModel": _device_get(self.device, "x-phonemodel", "Google, sdk_gphone_arm64"),
                "X-Platform": _device_get(self.device, "x-platform", "Android"),
            }
            if has_body:
                out["Content-Type"] = "application/json"
            return out

        if host == "accounts.goto-products.com":
            headers = {
                "Accept-Encoding": "gzip",
                "X-CVSDK-Version": os.environ.get("GOPAY_CVSDK_VERSION", "1.0.0"),
                "Gojek-Service-Area": "1",
                "X-Request-ID": str(uuid.uuid1()),
                "Country-Code": _device_get(self.device, "gojek-country-code", "ID"),
                "X-AppVersion": _device_get(self.device, "x-appversion", "2.7.0"),
                "X-M1": x_m1,
                "Gojek-Country-Code": _device_get(self.device, "gojek-country-code", "ID"),
                "X-UniqueId": _device_get(self.device, "x-uniqueid"),
                "X-PhoneMake": _device_get(self.device, "x-phonemake", "Google"),
                "X-Help-Version": _device_get(self.device, "x-appversion", "2.7.0"),
                "User-Agent": _device_get(self.device, "user-agent"),
                "X-DeviceOS": _device_get(self.device, "x-deviceos", "Android, 11"),
                "X-User-Type": _device_get(self.device, "x-user-type", "customer"),
                "X-AppId": _device_get(self.device, "x-appid", "com.gojek.gopay"),
                "Gojek-Timezone": os.environ.get("GOPAY_TIMEZONE", "Asia/Jakarta"),
                "X-AuthSDK-Version": os.environ.get("GOPAY_AUTHSDK_VERSION", "1.0.0"),
                "X-AppType": _device_get(self.device, "x-apptype", "GOPAY"),
                "X-User-Locale": os.environ.get("GOPAY_USER_LOCALE", "en_ID"),
                "X-DeviceToken": _device_get(self.device, "x-devicetoken"),
                "X-E2": _device_get(self.device, "x-e2", DEFAULT_X_E2),
                "Accept-Language": os.environ.get("GOPAY_ACCEPT_LANGUAGE", "en-ID"),
                "Transaction-ID": _device_get(self.device, "transaction-id"),
                "X-PhoneModel": _device_get(self.device, "x-phonemodel", "Google, sdk_gphone_arm64"),
                "X-Platform": _device_get(self.device, "x-platform", "Android"),
            }
            if has_body:
                headers["Content-Type"] = "application/json"
        elif host == "api.gojekapi.com" and (path in GOJEK_ACTIVITY_PATHS or path in GOJEK_APP_HEADER_PATHS):
            headers = app_headers()
        elif host == "customer.gopayapi.com" and _is_gopay_customer_link_path(path):
            headers = app_headers()
        elif host == "customer.gopayapi.com" and _is_gopay_customer_app_header_path(path):
            headers = app_headers()
        elif host == "customer.gopayapi.com" and method == "GET" and path in GOPAY_CUSTOMER_SLIM_GET_PATHS:
            headers = app_headers()
        else:
            headers.update(
                {
                    "User-uuid": _device_get(self.device, "user-uuid"),
                    "X-DeviceToken": _device_get(self.device, "x-devicetoken"),
                    "X-Location": _device_get(self.device, "x-location"),
                    "X-Location-Accuracy": _device_get(self.device, "x-location-accuracy"),
                    "Gojek-Country-Code": _device_get(self.device, "gojek-country-code", "ID"),
                    "X-Dark-Mode": "false",
                }
            )
        if path == "/api/v1/users/pin/tokens":
            headers["Sdk-Version"] = _device_get(self.device, "x-appversion", "2.7.0")
            headers["X-Biometric"] = ""
            headers["X-Verification"] = "PIN"
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}" if not self.token.startswith("Bearer") else self.token
        if extra_headers:
            headers.update(extra_headers)
        sign_token = _header_value(headers, "Authorization", self.token)
        xe1, _ = generate_xe1(method, url, body_str, sign_token, self.device, x_m1=x_m1)
        headers["X-E1"] = xe1
        return headers

    def _request(self, method: str, url: str, body: Optional[dict] = None, extra_headers: Optional[dict] = None) -> dict:
        body_str = _compact_json(body)
        headers = self._headers(method, url, body_str, extra_headers)
        try:
            resp = self.session.request(
                method,
                url,
                data=body_str.encode() if body_str else None,
                headers=headers,
                proxies=_proxy_map(self.proxy),
                timeout=15,
            )
            try:
                payload = resp.json()
            except ValueError:
                payload = {"raw": resp.text}
            data = payload
            if isinstance(payload, dict) and "data" in payload and payload.get("data") is not None:
                data = payload["data"]
            return {"status": resp.status_code, "data": data, "raw": payload}
        except requests.RequestException as e:
            return {"status": 0, "data": {"error": str(e)}}

    def get(self, url: str, **kwargs) -> dict:
        return self._request("GET", url, **kwargs)

    def post(self, url: str, body: Optional[dict] = None, **kwargs) -> dict:
        return self._request("POST", url, body=body, **kwargs)

    def patch(self, url: str, body: Optional[dict] = None, **kwargs) -> dict:
        return self._request("PATCH", url, body=body, **kwargs)

    def put(self, url: str, body: Optional[dict] = None, **kwargs) -> dict:
        return self._request("PUT", url, body=body, **kwargs)

    def delete(self, url: str, body: Optional[dict] = None, **kwargs) -> dict:
        return self._request("DELETE", url, body=body, **kwargs)
