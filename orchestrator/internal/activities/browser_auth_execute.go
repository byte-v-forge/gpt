package activities

import (
	"context"
	"errors"
	"fmt"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (f *browserAuthFlow) execute(client browserautomationv1.BrowserAutomationServiceClient, cfg BrowserAuthConfig, taskKey string, commands []*browserautomationv1.BrowserCommand) ([]*browserautomationv1.BrowserCommandResult, error) {
	sessionID := f.getSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("browser session is not ready")
	}
	labels := map[string]string{
		"domain":             "gpt",
		"workflow":           "browser_auth",
		"mode":               f.mode,
		"job_id":             f.jobID,
		"browser_session_id": sessionID,
	}
	if f.flowID != "" && f.flowID != sessionID {
		labels["flow_id"] = f.flowID
	}
	ctx, cancel := context.WithTimeout(f.ctx, taskTimeout(commands, cfg.CommandTimeout))
	defer cancel()
	resp, err := client.ExecuteBrowserCommands(ctx, &browserautomationv1.ExecuteBrowserCommandsRequest{
		RequestId: f.nextTaskRequestID(taskKey),
		Input: &browserautomationv1.BrowserTaskInput{
			SessionId:   sessionID,
			TaskKey:     "gpt.browser_auth." + taskKey,
			ScenarioKey: "gpt.browser_auth." + f.mode,
			Timeout:     durationpb.New(taskTimeout(commands, cfg.CommandTimeout)),
			Commands:    commands,
			Labels:      labels,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != nil {
		return resp.GetResults(), errors.New(resp.GetError().GetMessage())
	}
	continueOnError := map[string]bool{}
	for _, command := range commands {
		if command.GetContinueOnError() {
			continueOnError[command.GetCommandId()] = true
		}
	}
	for _, result := range resp.GetResults() {
		if result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_FAILED ||
			result.GetStatus() == browserautomationv1.BrowserCommandStatus_BROWSER_COMMAND_STATUS_TIMEOUT {
			if continueOnError[result.GetCommandId()] {
				continue
			}
			if result.GetError() != nil {
				return resp.GetResults(), errors.New(result.GetError().GetMessage())
			}
			return resp.GetResults(), fmt.Errorf("browser command %s failed", result.GetCommandKey())
		}
	}
	return resp.GetResults(), nil
}

func (f *browserAuthFlow) nextTaskRequestID(taskKey string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.taskSeq++
	if f.taskScope != "" {
		return fmt.Sprintf("gpt-browser-auth-%s-%s-%04d-%s", f.flowID, f.taskScope, f.taskSeq, taskKey)
	}
	return fmt.Sprintf("gpt-browser-auth-%s-%04d-%s", f.flowID, f.taskSeq, taskKey)
}
