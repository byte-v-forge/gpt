package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"orchestrator/db"
)

type accountProxyUsagesResponse struct {
	Usages []accountProxyUsageResponse `json:"usages"`
}

type accountProxyUsageResponse struct {
	ID             string                  `json:"id"`
	AccountID      string                  `json:"account_id"`
	JobID          string                  `json:"job_id"`
	N8NExecutionID string                  `json:"n8n_execution_id"`
	Purpose        string                  `json:"purpose"`
	ProxyURLHash   string                  `json:"proxy_url_hash"`
	ProxyProtocol  string                  `json:"proxy_protocol"`
	ProxyHost      string                  `json:"proxy_host"`
	ProxyPort      uint32                  `json:"proxy_port"`
	SessionIDHash  string                  `json:"session_id_hash"`
	ExitIP         string                  `json:"exit_ip"`
	CountryCode    string                  `json:"country_code"`
	Region         string                  `json:"region"`
	City           string                  `json:"city"`
	NetworkKind    string                  `json:"network_kind"`
	AnonymizerKind string                  `json:"anonymizer_kind"`
	FraudRiskLevel string                  `json:"fraud_risk_level"`
	FraudRiskScore float64                 `json:"fraud_risk_score"`
	EdgeRiskLevel  string                  `json:"edge_risk_level"`
	EdgeRiskScore  float64                 `json:"edge_risk_score"`
	AttemptIndex   uint32                  `json:"attempt_index"`
	Accepted       bool                    `json:"accepted"`
	ErrorMessage   string                  `json:"error_message"`
	CreatedAt      int64                   `json:"created_at"`
	Chain          proxyUsageChainResponse `json:"chain"`
}

type proxyUsageChainResponse struct {
	ChainID                  string `json:"chain_id"`
	LineSourceID             string `json:"line_source_id"`
	LineNodeID               string `json:"line_node_id"`
	LineDisplayName          string `json:"line_display_name"`
	DynamicProviderAccountID string `json:"dynamic_provider_account_id"`
	DynamicProviderID        string `json:"dynamic_provider_id"`
	DynamicGatewayID         string `json:"dynamic_gateway_id"`
	DynamicGatewayName       string `json:"dynamic_gateway_name"`
}

func (s *server) handleAccountProxyUsages(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		writeError(w, http.StatusBadGateway, errors.New("database is not configured"))
		return
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		writeError(w, http.StatusBadRequest, errors.New("account_id is required"))
		return
	}
	limit := queryLimit(r, 50, 200)
	var rows []db.AccountProxyUsage
	err := s.db.WithContext(r.Context()).
		Where("account_id = ?", accountID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	out := make([]accountProxyUsageResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, proxyUsageResponse(row))
	}
	writeJSON(w, http.StatusOK, accountProxyUsagesResponse{Usages: out})
}

func queryLimit(r *http.Request, fallback int, maxValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func proxyUsageResponse(row db.AccountProxyUsage) accountProxyUsageResponse {
	return accountProxyUsageResponse{
		ID:             row.ID,
		AccountID:      row.AccountID,
		JobID:          row.JobID,
		N8NExecutionID: row.N8NExecutionID,
		Purpose:        row.Purpose,
		ProxyURLHash:   row.ProxyURLHash,
		ProxyProtocol:  row.ProxyProtocol,
		ProxyHost:      row.ProxyHost,
		ProxyPort:      row.ProxyPort,
		SessionIDHash:  row.SessionIDHash,
		ExitIP:         row.ExitIP,
		CountryCode:    row.CountryCode,
		Region:         row.Region,
		City:           row.City,
		NetworkKind:    row.NetworkKind,
		AnonymizerKind: row.AnonymizerKind,
		FraudRiskLevel: row.FraudRiskLevel,
		FraudRiskScore: row.FraudRiskScore,
		EdgeRiskLevel:  row.EdgeRiskLevel,
		EdgeRiskScore:  row.EdgeRiskScore,
		AttemptIndex:   row.AttemptIndex,
		Accepted:       row.Accepted,
		ErrorMessage:   row.ErrorMessage,
		CreatedAt:      row.CreatedAt,
		Chain:          proxyUsageChain(row.RawJSON),
	}
}

func proxyUsageChain(raw string) proxyUsageChainResponse {
	var data map[string]any
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &data) != nil {
		return proxyUsageChainResponse{}
	}
	if chain, ok := data["chain"].(map[string]any); ok {
		return proxyUsageChainResponse{
			ChainID:                  dataString(chain, "chain_id"),
			LineSourceID:             dataString(chain, "line_source_id"),
			LineNodeID:               dataString(chain, "line_node_id"),
			LineDisplayName:          dataString(chain, "line_display_name"),
			DynamicProviderAccountID: dataString(chain, "dynamic_provider_account_id"),
			DynamicProviderID:        dataString(chain, "dynamic_provider_id"),
			DynamicGatewayID:         dataString(chain, "dynamic_gateway_id"),
			DynamicGatewayName:       dataString(chain, "dynamic_gateway_name"),
		}
	}
	return proxyUsageChainFromPlan(dataMap(data, "chain_plan"))
}

func proxyUsageChainFromPlan(plan map[string]any) proxyUsageChainResponse {
	line := dataMap(plan, "line")
	gateway := dataMap(plan, "dynamic_gateway")
	return proxyUsageChainResponse{
		ChainID:                  dataString(plan, "chain_id"),
		LineSourceID:             dataString(line, "source_id"),
		LineNodeID:               dataString(line, "node_id"),
		LineDisplayName:          dataString(line, "display_name"),
		DynamicProviderAccountID: dataString(gateway, "provider_account_id"),
		DynamicProviderID:        dataString(gateway, "provider_id"),
		DynamicGatewayID:         dataString(gateway, "gateway_id"),
		DynamicGatewayName:       dataString(gateway, "display_name"),
	}
}

func dataMap(data map[string]any, key string) map[string]any {
	if data == nil {
		return nil
	}
	value, _ := data[key].(map[string]any)
	return value
}

func dataString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
