# OpenAI Browser Login Session Steps

Source: manual browser-automation gRPC run against the remote Camoufox service.

Rules:
- Use exact CSS/role selectors from this document.
- Prefer browser-automation proto commands. Do not use page JS for the login path.
- Network request waits may be best-effort for this flow; the reliable success signal is the logged-in ChatGPT page followed by `/api/auth/session`.

## Manual run 2026-05-21

Test account domain: `edu.pood1e.space`

1. Start Camoufox Firefox with proxy ref `register`, locale `en-US`, timezone `America/New_York`, viewport `1365x768`.
   Result: session reached `BROWSER_SESSION_STATUS_RUNNING`.

2. Navigate directly to `https://chatgpt.com/auth/login`, optionally reject cookies, then wait `input#email[name="email"][type="email"][placeholder="Email address"][aria-label="Email address"]`.
   Result: `https://chatgpt.com/auth/login`, title `Get started | ChatGPT`, email input visible.

3. Fill the email input and click exact role selector `button[name="Continue"]`.
   Result: browser navigates to `https://auth.openai.com/email-verification`, title `Check your inbox - OpenAI`, with OTP input and `Continue with password` link.

4. Click exact role selector `link[name="Continue with password"]`.
   Result: `https://auth.openai.com/log-in/password`, title `Enter your password - OpenAI`, password input `input[name="current-password"][type="password"]`.

5. Fill `input[name="current-password"][type="password"]` and click exact role selector `button[name="Continue"]`.
   Result: browser either lands on `https://chatgpt.com/` with logged-in actions or enters the OpenAI callback URL `https://chatgpt.com/api/auth/callback/openai?...`; both mean the password submit step has advanced and the next step should wait for the session cookie.

6. Wait until the key ChatGPT session cookie is written, then navigate to `https://chatgpt.com/api/auth/session`, extract `body`, and parse JSON locally.
   Result: access token is read from the session API response; session token is read from the session cookie when the API response omits it. Device ID may still come from cookies.
