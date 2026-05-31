package activities

type loginStageStepData interface {
	setStage(stage string)
}

type sentinelStepData interface {
	setSentinelError(message string)
	setSentinelFlow(flow string)
	setSentinelTokenPresent(present bool)
	setSentinelTPresent(present bool)
}

type workspaceSelectStepData interface {
	setWorkspaceSelectError(message string)
	setWorkspaceSelectStatus(status int)
	setWorkspaceSelected(selected bool)
}

type chooseAccountStepData interface {
	setChooseAccountError(message string)
	setChooseAccountStatus(status int)
	setChooseAccountSelected(selected bool)
}

type codexOAuthProtocolNavigationData interface {
	loginStageStepData
	workspaceSelectStepData
	chooseAccountStepData
}

type chatGPTCSRFStepData interface {
	setChatGPTCSRFStatus(status int)
	setChatGPTCSRFAttempt(attempt int)
	setChatGPTCSRFEdgeChallenge(challenged bool)
	setChatGPTCSRFReady(ready bool)
}
