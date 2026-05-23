package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/browser-automation/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func navigateCommand(commandID, url string, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_Navigate{
			Navigate: &browserautomationv1.NavigateCommand{
				Url:       url,
				WaitUntil: browserautomationv1.BrowserNavigationWaitUntil_BROWSER_NAVIGATION_WAIT_UNTIL_DOM_CONTENT_LOADED,
				Timeout:   durationpb.New(timeout),
			},
		},
	}
}

func clickCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Click{
			Click: &browserautomationv1.ClickCommand{
				SelectorGroup: selectorGroup,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func fillCommand(commandID string, selector *browserautomationv1.BrowserSelector, value string, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Fill{
			Fill: &browserautomationv1.FillCommand{
				Selector: selector,
				Value:    value,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func fillGroupCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, value string, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Fill{
			Fill: &browserautomationv1.FillCommand{
				SelectorGroup: selectorGroup,
				Value:         value,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func pressCommand(commandID string, selector *browserautomationv1.BrowserSelector, key string, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_Press{
			Press: &browserautomationv1.PressCommand{
				Selector: selector,
				Key:      key,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func waitForLoadStateCommand(commandID string, state browserautomationv1.BrowserLoadState, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForLoadState{
			WaitForLoadState: &browserautomationv1.WaitForLoadStateCommand{
				State:   state,
				Timeout: durationpb.New(timeout),
			},
		},
	}
}

func waitForSelectorCommand(commandID string, selector *browserautomationv1.BrowserSelector, state browserautomationv1.BrowserSelectorState, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForSelector{
			WaitForSelector: &browserautomationv1.WaitForSelectorCommand{
				Selector: selector,
				State:    state,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func waitForSelectorGroupCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, state browserautomationv1.BrowserSelectorState, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForSelector{
			WaitForSelector: &browserautomationv1.WaitForSelectorCommand{
				SelectorGroup: selectorGroup,
				State:         state,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}

func typeTextCommand(commandID string, selector *browserautomationv1.BrowserSelector, text string, delay, timeout time.Duration, clearBefore, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_TypeText{
			TypeText: &browserautomationv1.TypeTextCommand{
				Selector:    selector,
				Text:        text,
				Delay:       durationpb.New(delay),
				Timeout:     durationpb.New(timeout),
				ClearBefore: clearBefore,
			},
		},
	}
}

func selectOptionGroupCommand(commandID string, selectorGroup *browserautomationv1.BrowserSelectorGroup, values, labels []string, indexes []int32, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_SelectOption{
			SelectOption: &browserautomationv1.SelectOptionCommand{
				SelectorGroup: selectorGroup,
				Values:        compactStringValues(values),
				Labels:        compactStringValues(labels),
				Indexes:       indexes,
				Timeout:       durationpb.New(timeout),
			},
		},
	}
}
