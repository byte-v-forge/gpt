package dashboard

import (
	"google.golang.org/protobuf/proto"

	"orchestrator/pb"
)

func n8nCodexOAuthWorkflowPayload(req *pb.CodexOAuthRequest, resp *pb.CodexOAuthResponse, accountID string) proto.Message {
	return &pb.N8NWorkflowTriggerPayload{JobId: resp.GetJobId(), AccountId: accountID, Label: req.GetLabel()}
}

func n8nCodexOAuthAddPhoneWorkflowPayload(req *pb.CodexOAuthAddPhoneRequest, resp *pb.CodexOAuthAddPhoneResponse, accountID string) proto.Message {
	return &pb.N8NWorkflowTriggerPayload{JobId: resp.GetJobId(), AccountId: accountID, Label: req.GetLabel(), MaxReuseCount: req.GetMaxReuseCount()}
}

func n8nCodexOAuthBatchAddPhoneWorkflowPayload(req *pb.CodexOAuthBatchAddPhoneRequest, resp *pb.CodexOAuthBatchAddPhoneResponse, _ string) proto.Message {
	return &pb.N8NWorkflowTriggerPayload{JobId: resp.GetJobId(), AccountIds: req.GetAccountIds(), Label: req.GetLabel(), MaxReuseCount: req.GetMaxReuseCount()}
}
