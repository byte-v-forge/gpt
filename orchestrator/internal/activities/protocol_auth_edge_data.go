package activities

import (
	"strings"

	"orchestrator/pb"
)

type protocolAuthEdgeStepData struct {
	message *pb.ActivityProtocolAuthEdgeData
}

func newProtocolAuthEdgeStepData(input ProtocolAuthStartInput, mode string) *protocolAuthEdgeStepData {
	return &protocolAuthEdgeStepData{message: &pb.ActivityProtocolAuthEdgeData{
		Driver:              "protocol",
		Mode:                strings.TrimSpace(mode),
		AccountId:           input.GetAccountId(),
		AuthEdgeCheckTarget: "chatgpt_csrf",
	}}
}

func (data *protocolAuthEdgeStepData) outputData() *pb.ActivityProtocolAuthOutputData {
	if data == nil {
		return nil
	}
	return protocolAuthOutputData(data.message)
}

func (data *protocolAuthEdgeStepData) setResult(accepted bool, err error) {
	if data == nil || data.message == nil {
		return
	}
	data.message.AuthEdgeAccepted = boolPtr(accepted)
	if err != nil {
		data.message.ErrorMessage = err.Error()
	} else {
		data.message.ErrorMessage = ""
	}
}

func (data *protocolAuthEdgeStepData) setChatGPTCSRFStatus(status int) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfStatus = int32(status)
	}
}

func (data *protocolAuthEdgeStepData) setChatGPTCSRFAttempt(attempt int) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfAttempt = int32(attempt)
	}
}

func (data *protocolAuthEdgeStepData) setChatGPTCSRFEdgeChallenge(challenged bool) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfEdgeChallenge = boolPtr(challenged)
	}
}

func (data *protocolAuthEdgeStepData) setChatGPTCSRFReady(ready bool) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfReady = boolPtr(ready)
	}
}
