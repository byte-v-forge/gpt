package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
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

func browserAuthBirthdaySelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(2*time.Second,
		cssSelector(`input[name*="birth" i],input[name*="dob" i],input[id*="birth" i],input[id*="dob" i],input[autocomplete="bday"],input[placeholder*="MM/DD" i],input[placeholder*="birth" i],input[aria-label*="birth" i],input[aria-label*="date of birth" i]`),
		labelSelector("Birthday", false),
		labelSelector("Date of birth", false),
		placeholderSelector("MM/DD/YYYY", false),
		roleSelector("textbox", "Birthday", false),
		roleSelector("textbox", "Date of birth", false),
	)
}

func browserAuthMonthInputSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`input[name*="month" i],input[id*="month" i],input[placeholder*="month" i],input[aria-label*="month" i]`),
		labelSelector("Month", false),
		placeholderSelector("Month", false),
		roleSelector("textbox", "Month", false),
	)
}

func browserAuthDayInputSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`input[name*="day" i],input[id*="day" i],input[placeholder*="day" i],input[aria-label*="day" i]`),
		labelSelector("Day", false),
		placeholderSelector("Day", false),
		roleSelector("textbox", "Day", false),
	)
}

func browserAuthYearInputSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`input[name*="year" i],input[id*="year" i],input[placeholder*="year" i],input[aria-label*="year" i]`),
		labelSelector("Year", false),
		placeholderSelector("Year", false),
		roleSelector("textbox", "Year", false),
	)
}

func browserAuthMonthSelectSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`select[name*="month" i],select[id*="month" i],select[aria-label*="month" i]`),
		labelSelector("Month", false),
	)
}

func browserAuthDaySelectSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`select[name*="day" i],select[id*="day" i],select[aria-label*="day" i]`),
		labelSelector("Day", false),
	)
}

func browserAuthYearSelectSelector() *browserautomationv1.BrowserSelectorGroup {
	return selectorGroup(time.Second,
		cssSelector(`select[name*="year" i],select[id*="year" i],select[aria-label*="year" i]`),
		labelSelector("Year", false),
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
