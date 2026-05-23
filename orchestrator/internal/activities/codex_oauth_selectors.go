package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func codexOAuthPhoneCountrySelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`select[name="country"],select[autocomplete="tel-country-code"],select[aria-label*="Country" i],select`),
	)
}

func codexOAuthPhoneCountryDropdownSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`button[aria-haspopup="listbox"],button[role="combobox"],button[aria-label*="Country" i]`),
	)
}

func codexOAuthPhoneCountryOptionSelector(labels []string) *browserautomationv1.BrowserSelectorGroup {
	selectors := make([]*browserautomationv1.BrowserSelector, 0, len(labels)*2)
	for _, label := range labels {
		selectors = append(selectors, roleSelector("option", label, true), textSelector(label, true))
	}
	return selectorGroup(2*time.Second, selectors...)
}

func codexOAuthPhoneInputSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[type="tel"][name="__reservedForPhoneNumberInput_tel"],input#tel[autocomplete="tel"],input[type="tel"]`)
}

func codexOAuthPhoneOTPSelector() *browserautomationv1.BrowserSelector {
	return cssSelector(`input[name="code"][autocomplete="one-time-code"][placeholder="Code"],input[autocomplete="one-time-code"],input[inputmode="numeric"]`)
}

func codexOAuthConsentSignalSelector() *browserautomationv1.BrowserSelector {
	return textSelector("Codex CLI", false)
}

func codexOAuthStageSelectorGroup(timeout time.Duration) *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(timeout,
		browserAuthLoginOTPSelector(),
		browserAuthLoginPasswordSelector(),
		codexOAuthPhoneInputSelector(),
		codexOAuthConsentSignalSelector(),
	)
}

func waitForURLCommand(commandID, pattern string, exact bool, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForUrl{
			WaitForUrl: &browserautomationv1.WaitForURLCommand{
				UrlPattern: pattern,
				Exact:      exact,
				Timeout:    durationpb.New(timeout),
			},
		},
	}
}
