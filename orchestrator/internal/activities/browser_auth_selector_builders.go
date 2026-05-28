package activities

import (
	"fmt"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func selectorGroup(timeout time.Duration, selectors ...*browserautomationv1.BrowserSelector) *browserautomationv1.BrowserSelectorGroup {
	return &browserautomationv1.BrowserSelectorGroup{
		Selectors: selectors,
		Timeout:   durationpb.New(timeout),
	}
}

func cssSelector(value string) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_CSS,
		Value: value,
	}
}

func textSelector(value string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_TEXT,
		Value: value,
		Exact: exact,
	}
}

func roleSelector(role, name string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:     browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_ROLE,
		Value:    name,
		RoleName: role,
		Exact:    exact,
	}
}

func labelSelector(value string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_LABEL,
		Value: value,
		Exact: exact,
	}
}

func placeholderSelector(value string, exact bool) *browserautomationv1.BrowserSelector {
	return &browserautomationv1.BrowserSelector{
		Kind:  browserautomationv1.BrowserSelectorKind_BROWSER_SELECTOR_KIND_PLACEHOLDER,
		Value: value,
		Exact: exact,
	}
}

func browserAuthLanguageOverrideScript(locale string) string {
	if locale == "" {
		locale = "en-US"
	}
	return fmt.Sprintf(`(() => {
  const language = %q;
  const languages = [language, "en"];
  const define = (object, property, value) => {
    try {
      Object.defineProperty(object, property, {get: () => value, configurable: true});
    } catch (_) {}
  };
  define(Navigator.prototype, "language", language);
  define(Navigator.prototype, "languages", languages);
})();`, locale)
}
