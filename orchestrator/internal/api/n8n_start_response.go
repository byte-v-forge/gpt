package api

import "orchestrator/pb"

type n8nStartResponseBuilder[R any] struct {
	Started func(string) R
	Failed  func(string, string) R
}

func n8nAccountStartResponse[R any](jobID string, accountID string, err error, builder n8nStartResponseBuilder[R]) (R, string, error) {
	response, startErr := n8nStartResponse(jobID, err, builder)
	return response, accountID, startErr
}

func n8nStartResponse[R any](jobID string, err error, builder n8nStartResponseBuilder[R]) (R, error) {
	if err != nil {
		return builder.Failed(jobID, err.Error()), err
	}
	return builder.Started(jobID), nil
}

var (
	n8nRegisterStartResponse = n8nStartResponseBuilder[*pb.RegisterAccountResponse]{
		Started: func(jobID string) *pb.RegisterAccountResponse {
			return &pb.RegisterAccountResponse{JobId: jobID, Started: true}
		},
		Failed: func(jobID string, errorMessage string) *pb.RegisterAccountResponse {
			return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: errorMessage}
		},
	}
	n8nLoginStartResponse = n8nStartResponseBuilder[*pb.LoginAccountResponse]{
		Started: func(jobID string) *pb.LoginAccountResponse {
			return &pb.LoginAccountResponse{JobId: jobID, Started: true}
		},
		Failed: func(jobID string, errorMessage string) *pb.LoginAccountResponse {
			return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: errorMessage}
		},
	}
	n8nCodexOAuthStartResponse = n8nStartResponseBuilder[*pb.CodexOAuthResponse]{
		Started: func(jobID string) *pb.CodexOAuthResponse {
			return &pb.CodexOAuthResponse{JobId: jobID, Started: true}
		},
		Failed: func(jobID string, errorMessage string) *pb.CodexOAuthResponse {
			return &pb.CodexOAuthResponse{JobId: jobID, ErrorMessage: errorMessage}
		},
	}
	n8nCodexOAuthAddPhoneStartResponse = n8nStartResponseBuilder[*pb.CodexOAuthAddPhoneResponse]{
		Started: func(jobID string) *pb.CodexOAuthAddPhoneResponse {
			return &pb.CodexOAuthAddPhoneResponse{JobId: jobID, Started: true}
		},
		Failed: func(jobID string, errorMessage string) *pb.CodexOAuthAddPhoneResponse {
			return &pb.CodexOAuthAddPhoneResponse{JobId: jobID, ErrorMessage: errorMessage}
		},
	}
	n8nCodexOAuthBatchAddPhoneStartResponse = n8nStartResponseBuilder[*pb.CodexOAuthBatchAddPhoneResponse]{
		Started: func(jobID string) *pb.CodexOAuthBatchAddPhoneResponse {
			return &pb.CodexOAuthBatchAddPhoneResponse{JobId: jobID, Started: true}
		},
		Failed: func(jobID string, errorMessage string) *pb.CodexOAuthBatchAddPhoneResponse {
			return &pb.CodexOAuthBatchAddPhoneResponse{JobId: jobID, ErrorMessage: errorMessage}
		},
	}
	n8nProbeStartResponse = n8nStartResponseBuilder[*pb.ProbeAccountResponse]{
		Started: func(jobID string) *pb.ProbeAccountResponse {
			return &pb.ProbeAccountResponse{JobId: jobID, Started: true}
		},
		Failed: func(jobID string, errorMessage string) *pb.ProbeAccountResponse {
			return &pb.ProbeAccountResponse{JobId: jobID, ErrorMessage: errorMessage}
		},
	}
)
