package activities

import (
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func waitForNetworkCommand(commandID string, urlSubstring string, method string, statusMin int32, statusMax int32, startedAfterUnixMs int64, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return waitForNetworkCommandWithContinue(commandID, urlSubstring, method, statusMin, statusMax, startedAfterUnixMs, timeout, false)
}

func waitForNetworkCommandWithContinue(commandID string, urlSubstring string, method string, statusMin int32, statusMax int32, startedAfterUnixMs int64, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_WaitForNetworkRequest{
			WaitForNetworkRequest: &browserautomationv1.WaitForNetworkRequestCommand{
				Filter: &browserautomationv1.BrowserNetworkRequestFilter{
					UrlSubstring:       urlSubstring,
					Method:             method,
					StatusCodeMin:      statusMin,
					StatusCodeMax:      statusMax,
					StartedAfterUnixMs: startedAfterUnixMs,
				},
				Timeout:         durationpb.New(timeout),
				RequireResponse: true,
			},
		},
	}
}

func submitFormCommand(commandID string, selector *browserautomationv1.BrowserSelector, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_SubmitForm{
			SubmitForm: &browserautomationv1.SubmitFormCommand{
				Selector: selector,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func countElementsCommand(commandID string, selector *browserautomationv1.BrowserSelector, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_CountElements{
			CountElements: &browserautomationv1.CountElementsCommand{
				Selector: selector,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func getPageStateCommand(commandID string, title, text, html bool, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_GetPageState{
			GetPageState: &browserautomationv1.GetPageStateCommand{
				IncludeTitle: title,
				IncludeText:  text,
				IncludeHtml:  html,
			},
		},
	}
}

func extractTextCommand(commandID string, selector *browserautomationv1.BrowserSelector, timeout time.Duration, continueOnError bool) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:       commandID,
		CommandKey:      commandID,
		Timeout:         durationpb.New(timeout),
		ContinueOnError: continueOnError,
		Operation: &browserautomationv1.BrowserCommand_ExtractText{
			ExtractText: &browserautomationv1.ExtractTextCommand{
				Selector: selector,
				Timeout:  durationpb.New(timeout),
			},
		},
	}
}

func getCookiesCommand(commandID string, urls []string, timeout time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(timeout),
		Operation: &browserautomationv1.BrowserCommand_GetCookies{
			GetCookies: &browserautomationv1.GetCookiesCommand{Urls: urls},
		},
	}
}

func waitTimeoutCommand(commandID string, duration time.Duration) *browserautomationv1.BrowserCommand {
	return &browserautomationv1.BrowserCommand{
		CommandId:  commandID,
		CommandKey: commandID,
		Timeout:    durationpb.New(duration + time.Second),
		Operation: &browserautomationv1.BrowserCommand_WaitForTimeout{
			WaitForTimeout: &browserautomationv1.WaitForTimeoutCommand{
				Duration: durationpb.New(duration),
			},
		},
	}
}

func taskTimeout(commands []*browserautomationv1.BrowserCommand, fallback time.Duration) time.Duration {
	timeout := fallback
	for _, command := range commands {
		if command.GetTimeout() == nil {
			continue
		}
		if commandTimeout := command.GetTimeout().AsDuration(); commandTimeout > timeout {
			timeout = commandTimeout
		}
	}
	return timeout + 15*time.Second
}
