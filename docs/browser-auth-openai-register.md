# OpenAI Browser Registration Steps

Source: manual browser-automation gRPC run against the remote Camoufox service.

Rules:
- Selectors are exact role/text selectors or exact attribute CSS selectors.
- Email polling starts from the network request `started_at_unix_ms` that submits the email form.
- After password registration returns to the email verification page, browser automation stops at an OTP-required state. The workflow waits for mailbox/manual OTP; auto mode may run a separate resend activity after the first 30 seconds, while manual mode only resends when the page triggers it.
- Business orchestration must follow the successful steps recorded below.

## Manual run 2026-05-20

Test mailbox domain: `edu.pood1e.space`

1. Start a browser-automation session with Camoufox Firefox, proxy ref `register`, locale `en-US`, timezone `America/New_York`, viewport `1365x768`, and labels `camoufox.geoip=true`, `camoufox.humanize=true`, `camoufox.disable_coop=true`, `camoufox.main_world_eval=false`.
   Result: session reached `BROWSER_SESSION_STATUS_RUNNING`.

2. Navigate directly to `https://chatgpt.com/auth/login` with `BROWSER_NAVIGATION_WAIT_UNTIL_DOM_CONTENT_LOADED`, then dismiss the cookie banner with exact role selector `button[name="Reject non-essential"]`.
   Result: `https://chatgpt.com/auth/login`, title `Get started | ChatGPT`, email input visible.

3. Fill exact CSS selector `input#email[name="email"][type="email"][placeholder="Email address"][aria-label="Email address"]`, then click exact role selector `button[name="Continue"]`.
   Result: `POST https://chatgpt.com/api/auth/signin/openai` returned HTTP `200`; request `started_at_unix_ms=1779286392758`. The browser then navigated to `https://auth.openai.com/email-verification` and showed exact title `Check your inbox - OpenAI` with input `input#_r_5_-code[name="code"][autocomplete="one-time-code"][placeholder="Code"]`.

4. On `https://auth.openai.com/email-verification`, click exact role selector `link[name="Continue with password"]`.
   Result: browser navigated to `https://auth.openai.com/create-account/password`, title `Create a password - OpenAI`, and showed exact password input `input[name="new-password"][autocomplete="new-password"][placeholder="Password"]`.
   If this lands on `https://auth.openai.com/log-in/password` with `input[name="current-password"][type="password"]`, treat the account as existing and switch to the login password flow instead of failing the registration.

5. Fill exact CSS selector `input[name="new-password"][autocomplete="new-password"][placeholder="Password"][type="password"]`, then click exact role selector `button[name="Continue"]`.
   Result: `POST https://auth.openai.com/api/accounts/user/register` returned HTTP `200`; request `started_at_unix_ms=1779286572673`. The browser then returned to `https://auth.openai.com/email-verification` with title `Check your inbox - OpenAI`.
   Note: password registration still requires the email verification code after account creation.

6. On `https://auth.openai.com/email-verification`, click exact role selector `button[name="Resend email"]`.
   Result: `POST https://auth.openai.com/api/accounts/email-otp/resend` returned HTTP `200`; request `started_at_unix_ms=1779287217363`. Use this request start timestamp as the lower bound when querying mailbox for the verification email.

7. Query mailbox for messages received after the resend request start timestamp, fill exact CSS selector `input#_r_5_-code[name="code"][autocomplete="one-time-code"][placeholder="Code"]`, then click exact role selector `button[name="Continue"]`.
   Result: `POST https://auth.openai.com/api/accounts/email-otp/validate` returned HTTP `200`; request `started_at_unix_ms=1779287270088`. The browser navigated to `https://auth.openai.com/about-you`, title `How old are you? - OpenAI`, with inputs `input[name="name"][autocomplete="name"][placeholder="Full name"]` and `input[name="age"][autocomplete="off"][placeholder="Age"]`.

8. Fill exact CSS selectors `input[name="name"][autocomplete="name"][placeholder="Full name"][type="text"]` and `input[name="age"][autocomplete="off"][placeholder="Age"][type="number"]`, then click exact role selector `button[name="Finish creating account"]`.
   Result: `POST https://auth.openai.com/api/accounts/create_account` returned HTTP `200`; request `started_at_unix_ms=1779287375878`. The subsequent ChatGPT OAuth callback landed on `https://chatgpt.com/auth/error?error=OAuthCallback`, and direct navigation back to `https://chatgpt.com/` showed the logged-out homepage. Account creation succeeded, but session establishment failed in this run.

## Orchestration update 2026-05-21

9. After `create_account` succeeds, wait only until the key ChatGPT session cookie is written.
   Result: then navigate to `https://chatgpt.com/api/auth/session` without waiting for the homepage UI. The access token is parsed from the session API body; the reusable session token is read from the browser session cookie when the API body does not include it.
