package activities

import (
	"google.golang.org/protobuf/proto"
	"orchestrator/internal/gptaccount"

	"orchestrator/internal/protowrap"
	"orchestrator/pb"
)

type protocolAuthStepData struct {
	message *pb.ActivityProtocolAuthStepData
}

func newProtocolAuthStepData(mode string, account *pb.Account) *protocolAuthStepData {
	data := &protocolAuthStepData{message: &pb.ActivityProtocolAuthStepData{
		Driver: "protocol",
		Mode:   mode,
	}}
	if account != nil {
		data.message.AccountId = gptaccount.ID(account)
		data.message.Email = gptaccount.Email(account)
	}
	return data
}

func protocolAuthStepMessage(data *protocolAuthStepData) *pb.ActivityProtocolAuthStepData {
	if data == nil {
		return &pb.ActivityProtocolAuthStepData{Driver: "protocol"}
	}
	return data.message
}

func protocolAuthOutputData(message proto.Message) *pb.ActivityProtocolAuthOutputData {
	out := &pb.ActivityProtocolAuthOutputData{}
	if !protowrap.SetMessage(out, message) {
		return nil
	}
	return out
}

func (data *protocolAuthStepData) setFlowID(flowID string) {
	if data != nil && data.message != nil {
		data.message.FlowId = stringAny(flowID)
	}
}

func (data *protocolAuthStepData) setStage(stage string) {
	if data != nil && data.message != nil {
		data.message.LoginStage = stringAny(stage)
	}
}

func (data *protocolAuthStepData) setProtocolSessionStarted(started bool) {
	if data != nil && data.message != nil {
		data.message.ProtocolSessionStarted = boolPtr(started)
	}
}

func (data *protocolAuthStepData) setEmailOTPIssuedAfter(issuedAfter int64) {
	if data != nil && data.message != nil {
		data.message.EmailOtpIssuedAfterUnix = issuedAfter
	}
}

func (data *protocolAuthStepData) setOTPSource(source string) {
	if data != nil && data.message != nil {
		data.message.OtpSource = stringAny(source)
	}
}

func (data *protocolAuthStepData) setChatGPTSignin(loginHint bool, screenHint string) {
	if data == nil || data.message == nil {
		return
	}
	data.message.ChatgptSigninLoginHint = boolPtr(loginHint)
	data.message.ChatgptSigninScreenHint = stringAny(screenHint)
}

func (data *protocolAuthStepData) setChatGPTAuthURLReady(ready bool) {
	if data != nil && data.message != nil {
		data.message.ChatgptAuthUrlReady = boolPtr(ready)
	}
}

func (data *protocolAuthStepData) setChatGPTCSRFStatus(status int) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfStatus = int32(status)
	}
}

func (data *protocolAuthStepData) setChatGPTCSRFAttempt(attempt int) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfAttempt = int32(attempt)
	}
}

func (data *protocolAuthStepData) setChatGPTCSRFEdgeChallenge(challenged bool) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfEdgeChallenge = boolPtr(challenged)
	}
}

func (data *protocolAuthStepData) setChatGPTCSRFReady(ready bool) {
	if data != nil && data.message != nil {
		data.message.ChatgptCsrfReady = boolPtr(ready)
	}
}

func (data *protocolAuthStepData) setEmailOTPSendError(message string) {
	if data != nil && data.message != nil {
		data.message.EmailOtpSendError = stringAny(message)
	}
}

func (data *protocolAuthStepData) setEmailOTPSendStatus(status int) {
	if data != nil && data.message != nil {
		data.message.EmailOtpSendStatus = int32(status)
	}
}

func (data *protocolAuthStepData) setProfilePresence(namePresent bool, birthdatePresent bool) {
	if data == nil || data.message == nil {
		return
	}
	data.message.ProfileNamePresent = boolPtr(namePresent)
	data.message.ProfileBirthdatePresent = boolPtr(birthdatePresent)
}

func (data *protocolAuthStepData) setCallbackURLCaptured(captured bool) {
	if data != nil && data.message != nil {
		data.message.CallbackUrlCaptured = boolPtr(captured)
	}
}

func (data *protocolAuthStepData) setSessionTokenPresent(present bool) {
	if data != nil && data.message != nil {
		data.message.SessionTokenPresent = boolPtr(present)
	}
}

func (data *protocolAuthStepData) setAccessTokenPresent(present bool) {
	if data != nil && data.message != nil {
		data.message.AccessTokenPresent = boolPtr(present)
	}
}

func (data *protocolAuthStepData) setSentinelError(message string) {
	if data != nil && data.message != nil {
		data.message.SentinelError = stringAny(message)
	}
}

func (data *protocolAuthStepData) setSentinelFlow(flow string) {
	if data != nil && data.message != nil {
		data.message.SentinelFlow = stringAny(flow)
	}
}

func (data *protocolAuthStepData) setSentinelTokenPresent(present bool) {
	if data != nil && data.message != nil {
		data.message.SentinelTokenPresent = boolPtr(present)
	}
}

func (data *protocolAuthStepData) setSentinelTPresent(present bool) {
	if data != nil && data.message != nil {
		data.message.SentinelTPresent = boolPtr(present)
	}
}

func (data *protocolAuthStepData) setWorkspaceSelectError(message string) {
	if data != nil && data.message != nil {
		data.message.WorkspaceSelectError = stringAny(message)
	}
}

func (data *protocolAuthStepData) setWorkspaceSelectStatus(status int) {
	if data != nil && data.message != nil {
		data.message.WorkspaceSelectStatus = int32(status)
	}
}

func (data *protocolAuthStepData) setWorkspaceSelected(selected bool) {
	if data != nil && data.message != nil {
		data.message.WorkspaceSelected = boolPtr(selected)
	}
}

func (data *protocolAuthStepData) setChooseAccountError(message string) {
	if data != nil && data.message != nil {
		data.message.ChooseAccountError = stringAny(message)
	}
}

func (data *protocolAuthStepData) setChooseAccountStatus(status int) {
	if data != nil && data.message != nil {
		data.message.ChooseAccountStatus = int32(status)
	}
}

func (data *protocolAuthStepData) setChooseAccountSelected(selected bool) {
	if data != nil && data.message != nil {
		data.message.ChooseAccountSelected = boolPtr(selected)
	}
}
