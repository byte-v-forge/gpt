package activities

import (
	"orchestrator/pb"
)

func browserStartData(resp *pb.StartRegisterResponse) *pb.ActivityBrowserStartData {
	if resp == nil {
		return &pb.ActivityBrowserStartData{ResponsePresent: boolPtr(false)}
	}
	return &pb.ActivityBrowserStartData{
		ResponsePresent:               boolPtr(true),
		Success:                       boolPtr(resp.GetSuccess()),
		ErrorMessage:                  resp.GetErrorMessage(),
		BrowserSessionId:              resp.GetBrowserSessionId(),
		OtpRequired:                   boolPtr(resp.GetOtpRequired()),
		OtpIssuedAfter:                resp.GetOtpIssuedAfterUnix(),
		OtpWaitStartedAtUnix:          resp.GetOtpWaitStartedAtUnix(),
		OtpRequestActionStartedAtUnix: resp.GetOtpRequestActionStartedAtUnix(),
		Stage:                         resp.GetStage(),
		StatusMessage:                 resp.GetStatusMessage(),
		Result:                        registerResultData(resp.GetResult()),
	}
}

func registerResultData(resp *pb.RegisterResponse) *pb.ActivityRegisterResultData {
	if resp == nil {
		return &pb.ActivityRegisterResultData{ResponsePresent: boolPtr(false)}
	}
	return &pb.ActivityRegisterResultData{
		ResponsePresent:        boolPtr(true),
		Success:                boolPtr(resp.GetSuccess()),
		ErrorMessage:           resp.GetErrorMessage(),
		SessionTokenPresent:    boolPtr(resp.GetSessionToken() != ""),
		AccessTokenPresent:     boolPtr(resp.GetAccessToken() != ""),
		DeviceIdPresent:        boolPtr(resp.GetDeviceId() != ""),
		PlusTrialEligible:      boolPtr(resp.GetPlusTrialEligible()),
		PlusTrialChecked:       boolPtr(resp.GetPlusTrialChecked()),
		CheckoutUrlPresent:     boolPtr(resp.GetCheckoutUrl() != ""),
		SensitiveValuesStored:  boolPtr(false),
		CredentialValuesStored: boolPtr(false),
	}
}

func plusTrialProbeData(resp *pb.ProbePlusTrialPaymentResponse) *pb.ActivityProbePlusTrialData {
	if resp == nil {
		return &pb.ActivityProbePlusTrialData{ResponsePresent: boolPtr(false)}
	}
	return &pb.ActivityProbePlusTrialData{
		ResponsePresent:    boolPtr(true),
		Success:            boolPtr(resp.GetSuccess()),
		ErrorMessage:       resp.GetErrorMessage(),
		Checked:            boolPtr(resp.GetChecked()),
		PlusTrialEligible:  boolPtr(resp.GetPlusTrialEligible()),
		PlusActive:         boolPtr(resp.GetPlusActive()),
		PlanType:           resp.GetPlanType(),
		Tier:               normalizeTier(resp.GetPlanType()),
		Amount:             resp.GetAmount(),
		Currency:           resp.GetCurrency(),
		Source:             resp.GetSource(),
		CheckoutUrlPresent: boolPtr(resp.GetCheckoutUrl() != ""),
		CheckoutSessionId:  resp.GetCheckoutSessionId(),
	}
}

func tierProbeData(resp *pb.ProbeTierPaymentResponse) *pb.ActivityProbeTierData {
	if resp == nil {
		return &pb.ActivityProbeTierData{ResponsePresent: boolPtr(false)}
	}
	return &pb.ActivityProbeTierData{
		ResponsePresent: boolPtr(true),
		Success:         boolPtr(resp.GetSuccess()),
		ErrorMessage:    resp.GetErrorMessage(),
		Checked:         boolPtr(resp.GetChecked()),
		Tier:            resp.GetTier(),
		PlusActive:      boolPtr(resp.GetPlusActive()),
		Source:          resp.GetSource(),
	}
}

func probePlusTrialStepData(accountID string, sessionToken string, accessToken string) *pb.ActivityProbePlusTrialStepData {
	return &pb.ActivityProbePlusTrialStepData{
		AccountId:           accountID,
		SessionTokenPresent: boolPtr(sessionToken != ""),
		AccessTokenPresent:  boolPtr(accessToken != ""),
	}
}

func applyPlusTrialProbeResponse(output *pb.ProbePlusTrialActivityOutput, data *pb.ActivityProbePlusTrialStepData, resp *pb.ProbePlusTrialPaymentResponse) {
	data.PaymentProbe = plusTrialProbeData(resp)
	if resp == nil {
		return
	}
	output.Success = resp.GetSuccess()
	output.Checked = resp.GetChecked()
	output.PlusTrialEligible = resp.GetPlusTrialEligible()
	output.PlusActive = resp.GetPlusActive()
	output.Amount = resp.GetAmount()
	output.Currency = resp.GetCurrency()
	output.Source = resp.GetSource()
	output.PlanType = resp.GetPlanType()
	output.CheckoutUrl = resp.GetCheckoutUrl()
	output.CheckoutSessionId = resp.GetCheckoutSessionId()
	output.ErrorMessage = resp.GetErrorMessage()
	data.Success = boolPtr(resp.GetSuccess())
	data.Checked = boolPtr(resp.GetChecked())
	data.PlusTrialEligible = boolPtr(resp.GetPlusTrialEligible())
	data.PlusActive = boolPtr(resp.GetPlusActive())
	data.PlanType = resp.GetPlanType()
	data.Tier = normalizeTier(resp.GetPlanType())
	data.Amount = resp.GetAmount()
	data.Currency = resp.GetCurrency()
	data.Source = resp.GetSource()
	data.CheckoutUrlPresent = boolPtr(resp.GetCheckoutUrl() != "")
	data.CheckoutSessionId = resp.GetCheckoutSessionId()
	data.ErrorMessage = resp.GetErrorMessage()
}

func probeTierStepData(accountID string, sessionToken string, accessToken string) *pb.ActivityProbeTierStepData {
	return &pb.ActivityProbeTierStepData{
		AccountId:           accountID,
		SessionTokenPresent: boolPtr(sessionToken != ""),
		AccessTokenPresent:  boolPtr(accessToken != ""),
	}
}

func applyTierProbeResponse(output *pb.ProbeTierActivityOutput, data *pb.ActivityProbeTierStepData, resp *pb.ProbeTierPaymentResponse) {
	data.TierProbe = tierProbeData(resp)
	if resp == nil {
		return
	}
	output.Success = resp.GetSuccess()
	output.Checked = resp.GetChecked()
	output.Tier = normalizeTier(resp.GetTier())
	output.PlusActive = resp.GetPlusActive()
	output.Source = resp.GetSource()
	output.ErrorMessage = resp.GetErrorMessage()
	data.Success = boolPtr(resp.GetSuccess())
	data.Checked = boolPtr(resp.GetChecked())
	data.Tier = output.Tier
	data.PlusActive = boolPtr(resp.GetPlusActive())
	data.Source = resp.GetSource()
	data.ErrorMessage = resp.GetErrorMessage()
}
