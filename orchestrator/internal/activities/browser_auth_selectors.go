package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func browserAuthEntrySelector(mode string) *browserautomationv1.BrowserSelectorGroup {
	if mode == "login" {
		return selectorGroup(3*time.Second,
			roleSelector("button", "Log in", false),
			roleSelector("link", "Log in", false),
			textSelector("Log in", false),
			roleSelector("button", "Sign in", false),
			roleSelector("link", "Sign in", false),
			textSelector("Sign in", false),
		)
	}
	return selectorGroup(3*time.Second,
		roleSelector("button", "Sign up for free", false),
		roleSelector("link", "Sign up for free", false),
		textSelector("Sign up for free", false),
		roleSelector("button", "Sign up", false),
		roleSelector("link", "Sign up", false),
		textSelector("Sign up", false),
		roleSelector("button", "Create account", false),
		roleSelector("link", "Create account", false),
		textSelector("Create account", false),
		roleSelector("button", "Get started", false),
		roleSelector("link", "Get started", false),
		textSelector("Get started", false),
		roleSelector("button", "Try ChatGPT", false),
		roleSelector("link", "Try ChatGPT", false),
		textSelector("Try ChatGPT", false),
	)
}

func browserAuthProfileMenuSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`button[aria-label="Open profile menu"],button[aria-label*="profile menu" i]`),
		roleSelector("button", "Open profile menu", true),
		textSelector("Open profile menu", true),
	)
}

func browserAuthEmailSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[type="email"],input[name*="email" i],input[id*="email" i],input[autocomplete="email"],input[placeholder*="email" i],input[aria-label*="email" i],input[name="username"],input[id*="username" i],input[autocomplete="username"],input[name="identifier"],input[id*="identifier" i],input[placeholder*="identifier" i],input[aria-label*="identifier" i]`)
}

func browserAuthPasswordSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[type="password"],input[name*="password" i],input[id*="password" i],input[autocomplete="current-password"],input[autocomplete="new-password"],input[placeholder*="password" i],input[aria-label*="password" i]`)
}

func browserAuthRegisterEmailSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input#email[name="email"][type="email"][placeholder="Email address"][aria-label="Email address"]`)
}

func browserAuthRejectCookiesSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		roleSelector("button", "Reject non-essential", true),
	)
}

func browserAuthRegisterPasswordSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="new-password"][autocomplete="new-password"][placeholder="Password"][type="password"]`)
}

func browserAuthRegisterOTPSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="code"][autocomplete="one-time-code"],input[name="code"],input[autocomplete="one-time-code"],input[placeholder="Code"],input[placeholder*="verification" i],input[aria-label*="code" i],input[id$="-code"],input[id*="code" i],input[data-testid*="code" i],input[data-testid*="otp" i]`)
}

func browserAuthLoginEmailAdvancedSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(5*time.Second,
		browserAuthLoginOTPSelector(),
		roleSelector("link", "Continue with password", true),
		browserAuthLoginPasswordSelector(),
	)
}

func browserAuthLoginPasswordSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="current-password"][type="password"]`)
}

func browserAuthLoginOTPSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="code"][autocomplete="one-time-code"][placeholder="Code"]`)
}

func browserAuthRegisterProfileNameSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="name"][autocomplete="name"][placeholder="Full name"][type="text"]`)
}

func browserAuthRegisterAgeSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="age"][autocomplete="off"][placeholder="Age"][type="number"]`)
}

func browserAuthEmailProviderSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		roleSelector("button", "Continue with email", false),
		textSelector("Continue with email", false),
		roleSelector("button", "Continue", true),
		textSelector("Continue", true),
		roleSelector("button", "Use email", false),
		textSelector("Use email", false),
		roleSelector("button", "Sign up with email", false),
		textSelector("Sign up with email", false),
		roleSelector("button", "Log in with email", false),
		textSelector("Log in with email", false),
	)
}

func browserAuthEmailSubmitSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(3*time.Second,
		roleSelector("button", "Continue", true),
		textSelector("Continue", true),
		roleSelector("button", "Next", true),
		textSelector("Next", true),
		roleSelector("button", "Sign up", true),
		textSelector("Sign up", true),
		roleSelector("button", "Create account", true),
		textSelector("Create account", true),
		roleSelector("button", "Log in", true),
		textSelector("Log in", true),
	)
}
