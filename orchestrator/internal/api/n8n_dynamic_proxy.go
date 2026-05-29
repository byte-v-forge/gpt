package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/gptsettings"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

type n8nDynamicProxySettings struct {
	JobID                  string         `json:"job_id"`
	AccountID              string         `json:"account_id"`
	N8NExecutionID         string         `json:"n8n_execution_id,omitempty"`
	Purpose                string         `json:"purpose"`
	CountryCode            string         `json:"country_code,omitempty"`
	Region                 string         `json:"region,omitempty"`
	State                  string         `json:"state,omitempty"`
	GeoSource              string         `json:"geo_source,omitempty"`
	PreflightEnabled       bool           `json:"preflight_enabled"`
	RequireResidential     bool           `json:"require_residential"`
	MinIPPurityScore       float64        `json:"min_ip_purity_score"`
	CFCanaryEnabled        bool           `json:"cf_canary_enabled"`
	MaxProxyAttempts       uint32         `json:"max_proxy_attempts"`
	TargetConnectivityURLs []string       `json:"target_connectivity_urls"`
	AuthEdgeCheckEnabled   bool           `json:"auth_edge_check_enabled"`
	AuthEdgeCheckTarget    string         `json:"auth_edge_check_target,omitempty"`
	LeaseRequest           map[string]any `json:"lease_request"`
}

type n8nDynamicProxyResult struct {
	JobID          string         `json:"job_id"`
	AccountID      string         `json:"account_id"`
	N8NExecutionID string         `json:"n8n_execution_id,omitempty"`
	Step           string         `json:"step"`
	Success        bool           `json:"success"`
	ProxyURL       string         `json:"proxy_url,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
}

func (s *Server) N8NDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	return s.n8nDynamicProxySettings(ctx, jobID, accountID, n8nExecutionID, actionRegisterProtocol, s.bindN8NRegisterProtocolExecution)
}

func (s *Server) N8NProbeDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string) (any, error) {
	return s.n8nDynamicProxySettings(ctx, jobID, accountID, n8nExecutionID, actionProbeAccount, s.bindN8NProbeExecution)
}

func (s *Server) n8nDynamicProxySettings(ctx context.Context, jobID string, accountID string, n8nExecutionID string, purpose string, bind func(context.Context, string, string) error) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := bind(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	params, err := s.jobStore.Params(ctx, jobID)
	if err != nil {
		return nil, err
	}
	settings := gptsettings.Default()
	if s.gptSettings != nil {
		stored, err := s.gptSettings.Get(ctx)
		if err != nil {
			return nil, err
		}
		settings = gptsettings.Normalize(stored)
	}
	preflight := settings.GetProxyPreflight()
	countryCode := normalizeProtocolCountryCode(params[protocolCountryCodeParam])
	region := normalizeProtocolRegion(countryCode, params[protocolRegionParam])
	geoSource := "job_params"
	if countryCode == "" || region == "" {
		countryCode, region, geoSource = s.dynamicProxyGeoFromFingerprint(ctx, accountID, countryCode, region)
	}
	if countryCode == "" || region == "" {
		err := fmt.Errorf("account country/region is required before acquiring dynamic proxy lease")
		data := map[string]any{
			"account_id":       accountID,
			"n8n_execution_id": n8nExecutionID,
			"country_code":     countryCode,
			"region":           region,
			"geo_source":       geoSource,
		}
		return nil, s.markActionFailed(ctx, jobID, stepDynamicIPPreflight, jobstatus.FailedRetryable, false, true, err, data)
	}
	state := protocolProxyState(countryCode, region)
	attempts := preflight.GetMaxProxyAttempts()
	if attempts == 0 {
		attempts = 10
	}
	authEdgeCheckEnabled := purpose == actionRegisterProtocol || purpose == actionLoginSessionProtocol
	request := map[string]any{
		"account_id": accountID,
		"purpose":    purpose,
		"force_new":  true,
		"policy": map[string]any{
			"mode":          "PROXY_SESSION_MODE_STICKY",
			"region":        countryCode,
			"state":         state,
			"sticky_ttl":    "600s",
			"upstream_kind": "PROXY_UPSTREAM_KIND_DYNAMIC_IP",
			"rotation_mode": "PROXY_ROTATION_MODE_STICKY_SESSION",
			"labels": map[string]string{
				"purpose":      purpose,
				"driver":       "gpt_workflow",
				"country_code": countryCode,
				"region":       region,
			},
		},
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{
		JobId:       jobID,
		StepName:    stepDynamicIPPreflight,
		Recoverable: false,
		Retryable:   true,
		Detail: structData(map[string]any{
			"account_id":               accountID,
			"n8n_execution_id":         n8nExecutionID,
			"purpose":                  purpose,
			"geo_source":               geoSource,
			"country_code":             countryCode,
			"region":                   region,
			"preflight_enabled":        preflight.GetEnabled(),
			"cf_canary_enabled":        preflight.GetCfCanaryEnabled(),
			"require_residential":      preflight.GetRequireResidential(),
			"min_ip_purity_score":      preflight.GetMinIpPurityScore(),
			"max_proxy_attempts":       attempts,
			"target_connectivity_urls": preflight.GetTargetConnectivityUrls(),
			"auth_edge_check_enabled":  authEdgeCheckEnabled,
			"auth_edge_check_target":   "chatgpt_csrf",
			"proxy_preflight_step":     "settings_loaded",
		}),
	}); err != nil {
		return nil, err
	}
	return &n8nDynamicProxySettings{
		JobID:                  jobID,
		AccountID:              accountID,
		N8NExecutionID:         n8nExecutionID,
		Purpose:                purpose,
		CountryCode:            countryCode,
		Region:                 region,
		State:                  state,
		GeoSource:              geoSource,
		PreflightEnabled:       preflight.GetEnabled(),
		RequireResidential:     preflight.GetRequireResidential(),
		MinIPPurityScore:       preflight.GetMinIpPurityScore(),
		CFCanaryEnabled:        preflight.GetCfCanaryEnabled(),
		MaxProxyAttempts:       attempts,
		TargetConnectivityURLs: preflight.GetTargetConnectivityUrls(),
		AuthEdgeCheckEnabled:   authEdgeCheckEnabled,
		AuthEdgeCheckTarget:    "chatgpt_csrf",
		LeaseRequest:           request,
	}, nil
}

func (s *Server) RecordN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error) {
	return s.recordN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, proxyURL, data, actionRegisterProtocol, s.bindN8NRegisterProtocolExecution)
}

func (s *Server) RecordN8NProbeDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any) (any, error) {
	return s.recordN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, proxyURL, data, actionProbeAccount, s.bindN8NProbeExecution)
}

func (s *Server) recordN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, data map[string]any, purpose string, bind func(context.Context, string, string) error) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := bind(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	proxyURL = strings.TrimSpace(proxyURL)
	parsed, err := url.Parse(proxyURL)
	if proxyURL == "" || err != nil || parsed.Scheme == "" || parsed.Host == "" {
		err = fmt.Errorf("proxy_runtime returned invalid proxy url")
		return nil, s.markActionFailed(ctx, jobID, stepDynamicIPPreflight, jobstatus.FailedRetryable, false, true, err, data)
	}
	if err := s.setJobParams(ctx, jobID, map[string]string{protocolProxyURLParam: proxyURL}); err != nil {
		return nil, err
	}
	if err := s.saveAccountProxyUsage(ctx, proxyUsageInput{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Purpose: purpose, ProxyURL: proxyURL, Data: data}); err != nil {
		return nil, err
	}
	if err := s.activities.StartJobStepActivity(ctx, pb.JobStepStartInput{JobId: jobID, StepName: stepDynamicIPPreflight, Recoverable: false, Retryable: true, Detail: structData(data)}); err != nil {
		return nil, err
	}
	if err := s.activities.CompleteJobStepActivity(ctx, pb.JobStepCompleteInput{
		JobId:       jobID,
		StepName:    stepDynamicIPPreflight,
		Recoverable: false,
		Retryable:   true,
		Result:      structData(data),
	}); err != nil {
		return nil, err
	}
	return &n8nDynamicProxyResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepDynamicIPPreflight, Success: true, ProxyURL: proxyURL, Data: data}, nil
}

func (s *Server) FailN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	return s.failN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, errorMessage, data, s.bindN8NRegisterProtocolExecution)
}

func (s *Server) FailN8NProbeDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any) (any, error) {
	return s.failN8NDynamicProxy(ctx, jobID, accountID, n8nExecutionID, errorMessage, data, s.bindN8NProbeExecution)
}

func (s *Server) failN8NDynamicProxy(ctx context.Context, jobID string, accountID string, n8nExecutionID string, errorMessage string, data map[string]any, bind func(context.Context, string, string) error) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := bind(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(errorMessage) == "" {
		errorMessage = "dynamic proxy preflight failed"
	}
	err := fmt.Errorf("%s", errorMessage)
	if markErr := s.markActionFailed(ctx, jobID, stepDynamicIPPreflight, jobstatus.FailedRetryable, false, true, err, data); markErr != nil {
		return nil, markErr
	}
	return &n8nDynamicProxyResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepDynamicIPPreflight, Data: data}, nil
}

func (s *Server) checkN8NProtocolAuthEdge(ctx context.Context, jobID string, accountID string, n8nExecutionID string, proxyURL string, mode string, bind func(context.Context, string, string) error) (any, error) {
	jobID, accountID, n8nExecutionID = normalizeN8NRegisterProtocolIDs(jobID, accountID, n8nExecutionID)
	if err := bind(ctx, jobID, n8nExecutionID); err != nil {
		return nil, err
	}
	proxyURL = strings.TrimSpace(proxyURL)
	base := map[string]any{
		"account_id":             accountID,
		"n8n_execution_id":       n8nExecutionID,
		"auth_edge_check_target": "chatgpt_csrf",
	}
	if proxyURL == "" {
		base["auth_edge_accepted"] = false
		base["error_message"] = "protocol proxy url is required"
		return &n8nDynamicProxyResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepProtocolAuthEdgeCheck, Success: false, Data: base}, nil
	}
	input := s.protocolAuthStartInput(ctx, jobID, accountID, mode)
	input.ProxyUrl = proxyURL
	out, err := s.activities.ProtocolAuthEdgeCheckActivity(ctx, input)
	data := structMap(out.GetData())
	for key, value := range base {
		if _, ok := data[key]; !ok {
			data[key] = value
		}
	}
	if err != nil {
		data["auth_edge_accepted"] = false
		data["error_message"] = err.Error()
		return &n8nDynamicProxyResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepProtocolAuthEdgeCheck, Success: false, ProxyURL: proxyURL, Data: data}, nil
	}
	data["auth_edge_accepted"] = true
	data["error_message"] = ""
	return &n8nDynamicProxyResult{JobID: jobID, AccountID: accountID, N8NExecutionID: n8nExecutionID, Step: stepProtocolAuthEdgeCheck, Success: true, ProxyURL: proxyURL, Data: data}, nil
}

func (s *Server) dynamicProxyGeoFromFingerprint(ctx context.Context, accountID string, countryCode string, region string) (string, string, string) {
	source := "account_fingerprint"
	if s.fingerprints == nil || strings.TrimSpace(accountID) == "" {
		return countryCode, region, "missing"
	}
	profile, ok, err := s.fingerprints.Get(ctx, accountID)
	if err != nil || !ok {
		return countryCode, region, "missing"
	}
	if countryCode == "" {
		countryCode = normalizeProtocolCountryCode(profile.CountryCode)
	}
	if region == "" {
		region = normalizeProtocolRegion(countryCode, profile.Region)
	}
	if countryCode == "" || region == "" {
		inferredCountry, inferredRegion, ok := accountfingerprint.GeoFromTimezone(profile.Timezone)
		if ok {
			source = "account_fingerprint_timezone"
			if countryCode == "" {
				countryCode = normalizeProtocolCountryCode(inferredCountry)
			}
			if region == "" {
				region = normalizeProtocolRegion(countryCode, inferredRegion)
			}
		}
	}
	return countryCode, region, source
}

func protocolProxyState(countryCode string, region string) string {
	countryCode = normalizeProtocolCountryCode(countryCode)
	region = normalizeProtocolRegion(countryCode, region)
	prefix, state, ok := strings.Cut(region, "-")
	if !ok || state == "" {
		return ""
	}
	if countryCode != "" && !strings.EqualFold(prefix, countryCode) {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(state))
}
