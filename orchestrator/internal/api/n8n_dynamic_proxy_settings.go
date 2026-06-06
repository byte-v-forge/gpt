package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	proxyruntimev1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/proxyruntime/v1"
	"google.golang.org/protobuf/types/known/durationpb"

	"orchestrator/internal/contracts"
	"orchestrator/internal/gptsettings"
	"orchestrator/pb"
)

type n8nDynamicProxyPreflightPlan struct {
	Settings *pb.N8NDynamicProxySettings
	Detail   *pb.N8NDynamicProxyPreflightDetail
}

func (s *Server) N8NDynamicProxySettings(ctx context.Context, actionID string, req *pb.N8NAuthStepRequest) (any, error) {
	profile, err := n8nDynamicProxyProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.n8nDynamicProxySettings(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), profile)
}

func (s *Server) n8nDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string, profile n8nDynamicProxyProfile) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NAuthIDs(jobID, accountID, n8nExecutionID)
	if err := s.bindN8NExecution(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	countryCode := normalizeProtocolCountryCode(params[protocolCountryCodeParam])
	region := normalizeProtocolRegion(countryCode, params[protocolRegionParam])
	geoSource := "job_params"
	if countryCode == "" || region == "" {
		countryCode, region, geoSource = s.dynamicProxyGeoFromFingerprint(ctx, accountID, countryCode, region)
	}
	if countryCode == "" || region == "" {
		err := fmt.Errorf("account country/region is required before acquiring dynamic proxy lease")
		data := n8nDynamicProxyGeoData(accountID, n8nExecutionID, countryCode, region, geoSource)
		return nil, s.markN8NDynamicProxyPreflightFailedMessage(ctx, jobID, err, data)
	}
	return s.n8nDynamicProxySettingsForGeo(ctx, jobID, accountID, n8nExecutionID, profile, countryCode, region, geoSource)
}

func (s *Server) n8nDynamicProxySettingsForGeo(ctx context.Context, jobID string, accountID string, n8nExecutionID string, profile n8nDynamicProxyProfile, countryCode string, region string, geoSource string) (any, error) {
	scope := n8nActionScopeFrom(jobID, accountID, n8nExecutionID)
	profile = profile.normalized()
	if profile.Purpose == "" {
		return nil, fmt.Errorf("dynamic proxy purpose is required")
	}
	countryCode = normalizeProtocolCountryCode(countryCode)
	region = normalizeProtocolRegion(countryCode, region)
	if strings.TrimSpace(geoSource) == "" {
		geoSource = "explicit"
	}
	if countryCode == "" {
		err := fmt.Errorf("proxy country is required before acquiring dynamic proxy lease")
		data := n8nDynamicProxyGeoData(scope.AccountID, scope.N8NExecutionID, countryCode, region, geoSource)
		return nil, s.markN8NDynamicProxyPreflightFailedMessage(ctx, scope.JobID, err, data)
	}
	plan, err := s.n8nDynamicProxyPreflightPlan(ctx, scope, profile, countryCode, region, geoSource)
	if err != nil {
		return nil, err
	}
	if err := s.activities.StartJobStepActivity(ctx, &pb.JobStepStartInput{
		JobId:       scope.JobID,
		StepName:    contracts.StepDynamicIPPreflight,
		Recoverable: false,
		Retryable:   true,
		Detail:      jobDataMessage(plan.Detail),
	}); err != nil {
		return nil, err
	}
	return plan.Settings, nil
}

func (s *Server) n8nDynamicProxyPreflightPlan(ctx context.Context, scope n8nActionScope, profile n8nDynamicProxyProfile, countryCode string, region string, geoSource string) (n8nDynamicProxyPreflightPlan, error) {
	settings, err := s.dynamicProxySettings(ctx)
	if err != nil {
		return n8nDynamicProxyPreflightPlan{}, err
	}
	preflight := settings.GetProxyPreflight()
	state := protocolProxyState(countryCode, region)
	attempts := preflight.GetMaxProxyAttempts()
	if attempts == 0 {
		attempts = 10
	}
	authEdgeCheckEnabled := profile.authEdgeCheckEnabled()
	settingsMessage := &pb.N8NDynamicProxySettings{
		JobId:                  scope.JobID,
		AccountId:              scope.AccountID,
		N8NExecutionId:         scope.N8NExecutionID,
		Purpose:                profile.Purpose,
		CountryCode:            countryCode,
		Region:                 region,
		State:                  state,
		GeoSource:              geoSource,
		PreflightEnabled:       preflight.GetEnabled(),
		RequireResidential:     preflight.GetRequireResidential(),
		MinIpPurityScore:       preflight.GetMinIpPurityScore(),
		CfCanaryEnabled:        preflight.GetCfCanaryEnabled(),
		MaxProxyAttempts:       attempts,
		TargetConnectivityUrls: preflight.GetTargetConnectivityUrls(),
		AuthEdgeCheckEnabled:   authEdgeCheckEnabled,
		AuthEdgeCheckTarget:    profile.AuthEdgeCheckTarget,
		LeaseRequest:           n8nDynamicProxyLeaseRequest(scope.AccountID, profile.Purpose, countryCode, region, state),
	}
	return n8nDynamicProxyPreflightPlan{
		Detail:   n8nDynamicProxyPreflightDetail(settingsMessage),
		Settings: settingsMessage,
	}, nil
}

func (s *Server) dynamicProxySettings(ctx context.Context) (*pb.GPTSettings, error) {
	if s.gptSettings == nil {
		return gptsettings.Default(), nil
	}
	stored, err := s.gptSettings.Get(ctx)
	if err != nil {
		return nil, err
	}
	return gptsettings.Normalize(stored), nil
}

func n8nDynamicProxyPreflightDetail(settings *pb.N8NDynamicProxySettings) *pb.N8NDynamicProxyPreflightDetail {
	return &pb.N8NDynamicProxyPreflightDetail{
		AccountId:              strings.TrimSpace(settings.GetAccountId()),
		N8NExecutionId:         strings.TrimSpace(settings.GetN8NExecutionId()),
		Purpose:                strings.TrimSpace(settings.GetPurpose()),
		GeoSource:              strings.TrimSpace(settings.GetGeoSource()),
		CountryCode:            strings.TrimSpace(settings.GetCountryCode()),
		Region:                 strings.TrimSpace(settings.GetRegion()),
		MaxProxyAttempts:       settings.GetMaxProxyAttempts(),
		TargetConnectivityUrls: settings.GetTargetConnectivityUrls(),
		ProxyPreflightStep:     "settings_loaded",
		PreflightEnabled:       settings.GetPreflightEnabled(),
		CfCanaryEnabled:        settings.GetCfCanaryEnabled(),
		RequireResidential:     settings.GetRequireResidential(),
		MinIpPurityScore:       settings.GetMinIpPurityScore(),
		AuthEdgeCheckEnabled:   settings.GetAuthEdgeCheckEnabled(),
		AuthEdgeCheckTarget:    strings.TrimSpace(settings.GetAuthEdgeCheckTarget()),
	}
}

func n8nDynamicProxyLeaseRequest(accountID string, purpose string, countryCode string, region string, state string) *proxyruntimev1.AcquireProxyLeaseRequest {
	purpose = strings.TrimSpace(purpose)
	countryCode = strings.TrimSpace(countryCode)
	region = strings.TrimSpace(region)
	return &proxyruntimev1.AcquireProxyLeaseRequest{
		AccountId: strings.TrimSpace(accountID),
		Purpose:   purpose,
		ForceNew:  true,
		Policy: &proxyruntimev1.ProxySessionPolicy{
			Mode:         proxyruntimev1.ProxySessionMode_PROXY_SESSION_MODE_STICKY,
			Region:       countryCode,
			State:        strings.TrimSpace(state),
			StickyTtl:    durationpb.New(10 * time.Minute),
			UpstreamKind: proxyruntimev1.ProxyUpstreamKind_PROXY_UPSTREAM_KIND_DYNAMIC_IP,
			RotationMode: proxyruntimev1.ProxyRotationMode_PROXY_ROTATION_MODE_STICKY_SESSION,
			Labels: map[string]string{
				"purpose":      purpose,
				"driver":       "gpt_workflow",
				"country_code": countryCode,
				"region":       region,
			},
		},
		SelectionPolicy: &proxyruntimev1.ProxyDynamicIPSelectionPolicy{
			CountryCode: countryCode,
			Region:      region,
			Purpose:     purpose,
		},
	}
}
