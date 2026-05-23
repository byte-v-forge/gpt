# GoPay protocol notes from DanOps-1 PR #28

Source: `DanOps-1/Gpt-Agreement-Payment` PR #28, file `output/gopay_2.8.0_extract/REVERSE_KNOWLEDGE_BASE.md`.

Corrections applied here:

- Midtrans QRIS charge still starts at `app.midtrans.com/snap/v2/transactions/{snap}/charge`.
- QRIS charge payload carries `gross_amount: "1"` to avoid zero-IDR GoPay rejection; tokenized GoPay charge keeps DanOps' `payment_type=gopay, tokenization=true` shape.
- QRIS-to-payment settlement should use GoPay gateway endpoints, not the older QR capture flow:
  1. `GET https://gwa.gopayapi.com/v1/payment/validate?reference_id={A...ID}`
  2. `POST https://gwa.gopayapi.com/v1/payment/confirm?reference_id={A...ID}` with `{payment_instructions: []}`
  3. `POST https://customer.gopayapi.com/api/v1/users/pin/tokens/nb` with `{challenge_id, client_id, pin}`
  4. `POST https://gwa.gopayapi.com/v1/payment/process?reference_id={A...ID}` with `GOPAY_PIN_CHALLENGE`.
- `gwa.gopayapi.com` requests use merchant web origin/referrer and do not use `X-E1`.
- GoPay app default headers now target app version `2.8.0` / build `2080`.
- GoPay app device headers now keep the PR #28 captured app/profile shape, but generate per-device persisted device identity and lower hardware/network IDs (`x-uniqueid`, `D1`, MediaDrm/Widevine, WiFi BSSID/SSID, `m1_connection_id`, `m1_signature`, `m1_signature_time`, `m1_device_uuid`, AppsFlyer, Firebase app instance ID, FCM-like `X-DeviceToken`, AD ID/App Set ID metadata, `X-IMEI`, and `X-IpAddress`). The generated device also binds a realistic Android make/model/screen profile and uses APK-observed no-space forms such as `Android,16` and `Redmi,23117RK66C`. Use `GOPAY_STATIC_DEVICE_IDENTITY=1` only for deterministic protocol debugging; do not use it for signup/payment flows because a globally reused device identity is easy to correlate.
- `X-M1` follows the 2.8.0 string labels observed in the APK: `3:appsflyerId`, `6:wifiMac`, `7:wifiSSID`, `8:screen`, `9:locationMethod`, `11:widevineId`, `13:signature`, `14:signatureTime`, `15:firebaseAppInstanceId`, `16:deviceUUID`.
- A connected Android device profile can be exported with `scripts/gopay-adb-device-profile.sh` and sourced into the app-service environment. The script only reads non-secret device/profile fields; `X-DeviceToken` is app-private and is generated when `GOPAY_DEVICE_TOKEN` is not provided.
- Default location is aligned with `Gojek-Timezone=Asia/Jakarta` / `Gojek-Country-Code=ID`; override `GOPAY_LOCATION` and `GOPAY_LOCATION_ACCURACY` when replaying a fresh capture.
- Dynamic egress is configured with `GOPAY_DYNAMIC_EGRESS`; the old `GOPAY_PROXY_POOL` name is intentionally not kept as a fallback. When `PROXY_RUNTIME_HTTP_ADDR` is present, `GenerateDeviceProxy` actively creates a fresh sticky ID session before phone/signup probing instead of waiting for upstream rotation.
- Signup now intentionally avoids a fixed machine-speed sequence: the payment workflow waits a recorded random 8-25 seconds before the signup child workflow, and the GoPay app service waits another configurable random interval before `/cvs/v1/initiate` (`GOPAY_SIGNUP_INITIATE_JITTER_MIN_SECONDS`, `GOPAY_SIGNUP_INITIATE_JITTER_MAX_SECONDS`, defaults `8..25`). A signup rate-limit response marks the current device/egress/phone state with `_signup_cooldown_until` (`GOPAY_SIGNUP_RATE_LIMIT_COOLDOWN_SECONDS`, default `900`) so the same generated state is not immediately reused.
- GoPay app signing defaults to endpoint-aware `auto`: legacy v1 for auth/CVS/login endpoints, v2 for confirmed v2 endpoints such as `customer.gopayapi.com/api/v1/users/pin/tokens/nb`. Override with `GOPAY_SIGN_VERSION=v1|v2` only for focused debugging.
- PR #28 notes do not mention TLS/JA3/ClientHello details. They only mention server-side device fingerprinting and `x-m1`/device fields. The app now binds one Android TLS profile when a device/proxy state is generated and reuses it for that state. Useful envs:
  - `GOPAY_TLS_PROFILE=okhttp4_android_12` pins a profile.
  - `GOPAY_TLS_PROFILES=okhttp4_android_12,okhttp4_android_13` limits the random pool.
  - `GOPAY_TLS_RANDOM_EXTENSION_ORDER=1` enables experimental per-connection TLS extension order randomization; default is off to keep device-level TLS behavior stable.
  - `GOPAY_TLS_FORCE_HTTP1=1` restores forced HTTP/1.1 if needed.
- DanOps QRIS/GoPay payment code uses a Chrome-like HTTP stack (`curl_cffi` when available) and fresh UUIDs in Stripe/Midtrans/GoPay request bodies/headers, but its QRIS `charge` attempts share one external session. This repo intentionally rotates payment-attempt fingerprints for both QRIS and WA/tokenization paths: Midtrans linking, GWA linking/OTP/PIN/payment calls, and `snap/v2/.../charge` rebuild the TLS client with a different Chrome profile/user-agent while keeping the cookie jar, and send fresh `x-device-id`, `x-correlation-id`, and `x-request-id`.
- The ChatGPT/Stripe front half follows DanOps where the current Stripe API accepts it: checkout defaults to `custom`, Stripe PM/confirm include manual-approval runtime attribution fields, redirect extraction accepts setup/payment/invoice intents, and checkout approve uses a clean minimal session/header set instead of the long-lived browser session defaults. DanOps-only fields rejected by current Stripe endpoints, such as `elements_options_client[stripe_js_locale]` on `/init`, are intentionally not sent.
- The plus-trial probe may create a checkout only to inspect eligibility, but GoPay payment prepare intentionally creates a fresh checkout in the same payment activity/session. Reusing the probe checkout across activities changes browser/device context before `checkout/approve` and can trigger `{"result":"blocked"}`. The activity layer also ignores any already-scheduled probe checkout URL/session ID so retries of older workflow attempts do not keep reusing stale checkout state.
- Before creating a payment checkout, this repo now performs the DanOps-style ChatGPT warm-up sequence (`/`, auth session, account checks, pricing config, plus home-bounce backend endpoints) and merges warmed ChatGPT cookies into the dedicated approve request. The checkout response processor entity is also propagated into approve/verify instead of hard-coding it, and an approve `blocked` result triggers one fresh-checkout retry. The dedicated approve session still sends the same Chrome User-Agent/language/client-hint headers because tls-client does not auto-populate them the way DanOps' curl_cffi impersonation does; without those headers ChatGPT can return a 403 HTML challenge before business anti-fraud runs.
- For WA/tokenization, DanOps treats `snap/v2/.../charge` denial after successful linking/PIN validation as `linking_only` because Stripe/Midtrans webhook settlement might happen asynchronously. Local runs did not show useful settlement after `deny`, so this repo now treats Midtrans `deny`/fraud denial as an immediate payment failure instead of waiting or marking `async_settlement_pending`.

Useful local probes:

```bash
python3 scripts/probe-gopay-v2-signature.py --self-test
GOPAY_SSO_TOKEN=... GOPAY_SIGNED_MSG_TEMPLATE=/tmp/big_msg_1867.bin \
  python3 scripts/probe-gopay-v2-signature.py --send
```

`--self-test` verifies the deterministic v2 cipher vector from PR #28. Full server acceptance still requires a fresh SSO token and a captured `signed_msg` template matching the device fields.
