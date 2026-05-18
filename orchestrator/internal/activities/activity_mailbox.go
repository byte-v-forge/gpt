package activities

import (
	"context"
	"fmt"
	"orchestrator/pb"
	"strings"
)

func (s *Server) RegisterMailboxAtomicActivity(ctx context.Context, input MailboxRegistrationActivityInput) (MailboxRegistrationActivityOutput, error) {
	var output MailboxRegistrationActivityOutput
	step := s.activityStep(ctx, input.GetJobId(), stepRegisterMailbox, false, true)
	_, err := step.run(func() (any, error) {
		resp, callErr := s.mailboxRegisterClient.RunMailboxRegistration(ctx, &pb.RunMailboxRegistrationRequest{
			Enabled:    input.GetEnabled(),
			ImportOnly: input.GetImportOnly(),
		})
		data := map[string]any{
			"enabled":        input.GetEnabled(),
			"import_only":    input.GetImportOnly(),
			"mailboxes":      []map[string]any{},
			"account_count":  0,
			"imported_count": 0,
		}
		defer func() { output.Data = protoData(data) }()
		if resp != nil {
			output.Success = resp.GetSuccess()
			output.ExitCode = resp.GetExitCode()
			output.ErrorMessage = resp.GetErrorMessage()
			data["success"] = resp.GetSuccess()
			data["exit_code"] = resp.GetExitCode()
			data["error_message"] = resp.GetErrorMessage()
			data["account_count"] = len(resp.GetAccounts())
		}
		if callErr != nil {
			return data, callErr
		}
		if resp == nil {
			return data, fmt.Errorf("mailbox registration service returned empty response")
		}
		if !resp.GetSuccess() {
			msg := resp.GetErrorMessage()
			if msg == "" {
				msg = fmt.Sprintf("mailbox registration failed with exit code %d", resp.GetExitCode())
			}
			return data, fmt.Errorf("%s", msg)
		}
		if len(resp.GetAccounts()) == 0 {
			msg := "mailbox registration returned no accounts"
			output.Success = false
			output.ErrorMessage = msg
			data["success"] = false
			data["error_message"] = msg
			return data, fmt.Errorf("%s", msg)
		}

		imported := make([]map[string]any, 0, len(resp.GetAccounts()))
		for _, account := range resp.GetAccounts() {
			email := strings.ToLower(strings.TrimSpace(account.GetEmailAddress()))
			password := strings.TrimSpace(account.GetPassword())
			if email == "" {
				msg := "mailbox registration returned account without email"
				output.Success = false
				output.ErrorMessage = msg
				data["success"] = false
				data["error_message"] = msg
				return data, fmt.Errorf("%s", msg)
			}
			if password == "" {
				msg := fmt.Sprintf("mailbox registration returned %s without password", email)
				output.Success = false
				output.ErrorMessage = msg
				data["success"] = false
				data["error_message"] = msg
				return data, fmt.Errorf("%s", msg)
			}

			refreshToken := strings.TrimSpace(account.GetRefreshToken())
			authStatus := emailAuthStatusAuthorized
			if refreshToken == "" {
				authStatus = emailAuthStatusOAuthPending
			}
			allocationStatus := gptAllocationStatusForMailboxAuth(authStatus)
			allocResp, allocErr := s.accountClient.UpsertGPTEmailAllocation(ctx, &pb.UpsertGPTEmailAllocationRequest{
				Allocation: &pb.GPTEmailAllocation{
					Email:        email,
					PrimaryEmail: email,
					IsPrimary:    true,
					Status:       allocationStatus,
					Splittable:   false,
					LastError:    "",
				},
			})
			if allocErr != nil {
				output.Success = false
				output.ErrorMessage = allocErr.Error()
				data["success"] = false
				data["error_message"] = allocErr.Error()
				return data, allocErr
			}
			if allocResp.GetAllocation() != nil && strings.TrimSpace(allocResp.GetAllocation().GetStatus()) != "" {
				allocationStatus = allocResp.GetAllocation().GetStatus()
			}
			importedMailbox := RegisteredMailboxResult{
				EmailAddress: email,
				Status:       allocationStatus,
			}
			output.Mailboxes = append(output.Mailboxes, &importedMailbox)
			imported = append(imported, map[string]any{
				"email_address": importedMailbox.GetEmailAddress(),
				"status":        importedMailbox.GetStatus(),
			})
		}
		output.Success = len(output.Mailboxes) > 0
		data["success"] = output.Success
		data["imported_count"] = len(output.Mailboxes)
		data["mailboxes"] = imported
		return data, nil
	})
	if err != nil {
		return output, err
	}
	return output, nil
}

func (s *Server) MailboxOAuthAtomicActivity(ctx context.Context, input MailboxOAuthActivityInput) (MailboxOAuthActivityOutput, error) {
	var output MailboxOAuthActivityOutput
	step := s.activityStep(ctx, input.GetJobId(), stepMailboxOAuth, false, true)
	_, err := step.run(func() (any, error) {
		data := map[string]any{
			"email_address": strings.TrimSpace(input.GetEmailAddress()),
			"only_missing":  input.GetOnlyMissing(),
			"limit":         input.GetLimit(),
			"results":       []map[string]any{},
		}
		defer func() { output.Data = protoData(data) }()
		resp, callErr := s.mailboxRegisterClient.RunMailboxOAuth(ctx, &pb.RunMailboxOAuthRequest{
			EmailAddress: strings.TrimSpace(input.GetEmailAddress()),
			OnlyMissing:  input.GetOnlyMissing(),
			Limit:        input.GetLimit(),
		})
		if resp != nil {
			output.Success = resp.GetSuccess()
			output.Processed = resp.GetProcessed()
			output.Succeeded = resp.GetSucceeded()
			output.Failed = resp.GetFailed()
			output.ErrorMessage = resp.GetErrorMessage()
			results := make([]map[string]any, 0, len(resp.GetResults()))
			for _, item := range resp.GetResults() {
				results = append(results, map[string]any{
					"email_address":     item.GetEmailAddress(),
					"success":           item.GetSuccess(),
					"error_message":     item.GetErrorMessage(),
					"has_refresh_token": strings.TrimSpace(item.GetRefreshToken()) != "",
				})
			}
			data["success"] = resp.GetSuccess()
			data["processed"] = resp.GetProcessed()
			data["succeeded"] = resp.GetSucceeded()
			data["failed"] = resp.GetFailed()
			data["error_message"] = resp.GetErrorMessage()
			data["results"] = results
		}
		if callErr != nil {
			return data, callErr
		}
		if resp == nil {
			return data, fmt.Errorf("mailbox registration service returned empty OAuth response")
		}
		for _, item := range resp.GetResults() {
			email := strings.ToLower(strings.TrimSpace(item.GetEmailAddress()))
			if email == "" {
				continue
			}
			refreshToken := strings.TrimSpace(item.GetRefreshToken())
			if item.GetSuccess() && refreshToken != "" {
				if _, upsertErr := s.accountClient.UpsertGPTEmailAllocation(ctx, &pb.UpsertGPTEmailAllocationRequest{
					Allocation: &pb.GPTEmailAllocation{
						Email:        email,
						PrimaryEmail: email,
						IsPrimary:    true,
						Status:       emailStatusAvailable,
						Splittable:   false,
						LastError:    "",
					},
				}); upsertErr != nil {
					output.Success = false
					output.ErrorMessage = upsertErr.Error()
					data["success"] = false
					data["error_message"] = upsertErr.Error()
					return data, upsertErr
				}
				continue
			}
			if email != "" && !item.GetSuccess() {
				errorMessage := strings.TrimSpace(item.GetErrorMessage())
				_, _ = s.accountClient.UpsertGPTEmailAllocation(ctx, &pb.UpsertGPTEmailAllocationRequest{
					Allocation: &pb.GPTEmailAllocation{
						Email:        email,
						PrimaryEmail: email,
						IsPrimary:    true,
						Status:       mailboxOAuthFailureStatus(errorMessage),
						Splittable:   false,
						LastError:    errorMessage,
					},
				})
			}
		}
		if !resp.GetSuccess() {
			msg := resp.GetErrorMessage()
			if msg == "" {
				msg = fmt.Sprintf("mailbox OAuth failed: %d/%d", resp.GetFailed(), resp.GetProcessed())
			}
			output.ErrorMessage = msg
			data["error_message"] = msg
			return data, fmt.Errorf("%s", msg)
		}
		return data, nil
	})
	if err != nil {
		return output, err
	}
	return output, nil
}

func mailboxOAuthFailureStatus(errorMessage string) string {
	errorText := strings.ToLower(strings.TrimSpace(errorMessage))
	if strings.Contains(errorText, "needs_manual_verification") || strings.Contains(errorText, "account.live.com/abuse") {
		return emailAuthStatusNeedsManualVerify
	}
	return emailAuthStatusAuthFailed
}

func gptAllocationStatusForMailboxAuth(authStatus string) string {
	switch strings.TrimSpace(authStatus) {
	case emailAuthStatusAuthorized:
		return emailStatusAvailable
	case emailAuthStatusAuthFailed, emailAuthStatusNeedsManualVerify:
		return strings.TrimSpace(authStatus)
	default:
		return emailStatusOAuthPending
	}
}
