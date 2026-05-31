package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
)

func browserAuthProfileNameSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`input[name="name"],input[name*="full" i],input[name*="display" i],input[id*="name" i],input[autocomplete="name"],input[placeholder*="name" i],input[aria-label*="name" i]`),
		labelSelector("Full name", false),
		labelSelector("Name", true),
		placeholderSelector("Full name", false),
		placeholderSelector("Name", true),
		roleSelector("textbox", "Full name", false),
		roleSelector("textbox", "Name", true),
	)
}

func browserAuthAgeSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`input[name*="age" i],input[id*="age" i],input[placeholder*="age" i],input[aria-label*="age" i],input[type="number"]`),
		labelSelector("Age", false),
		placeholderSelector("Age", false),
		roleSelector("spinbutton", "Age", false),
		roleSelector("textbox", "Age", false),
	)
}

func browserAuthPostOTPContinueSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(3*time.Second,
		cssSelector(`button[type="submit"],input[type="submit"]`),
		roleSelector("button", "Continue", true),
		textSelector("Continue", true),
		roleSelector("button", "Next", true),
		textSelector("Next", true),
		roleSelector("button", "Finish", true),
		textSelector("Finish", true),
		roleSelector("button", "Finish creating account", true),
		textSelector("Finish creating account", true),
		roleSelector("button", "Create", true),
		textSelector("Create", true),
		roleSelector("button", "Agree", true),
		textSelector("Agree", true),
	)
}
