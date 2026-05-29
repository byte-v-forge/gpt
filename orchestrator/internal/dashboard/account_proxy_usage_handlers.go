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
	ID              string                  `json:"id"`
	AccountID       string                  `json:"account_id"`
	JobID           string                  `json:"job_id"`
	N8NExecutionID  string                  `json:"n8n_execution_id"`
	Purpose         string                  `json:"purpose"`
	ProxyURLHash    string                  `json:"proxy_url_hash"`
	SessionIDHash   string                  `json:"session_id_hash"`
	ExitIP          string                  `json:"exit_ip"`
	CountryCode     string                  `json:"country_code"`
	Region          string                  `json:"region"`
	City            string                  `json:"city"`
	IPFraudCheck    map[string]any          `json:"ip_fraud_check"`
	EdgeAccessCheck map[string]any          `json:"edge_access_check"`
	TargetReachable bool                    `json:"target_reachable"`
	AttemptIndex    uint32                  `json:"attempt_index"`
	Accepted        bool                    `json:"accepted"`
	ErrorMessage    string                  `json:"error_message"`
	CreatedAt       int64                   `json:"created_at"`
	Chain           proxyUsageChainResponse `json:"chain"`
}

type proxyUsageChainResponse struct {
	ChainID string                       `json:"chain_id"`
	Hops    []proxyUsageChainHopResponse `json:"hops"`
}

type proxyUsageChainHopResponse struct {
	HopID              string `json:"hop_id"`
	Order              uint32 `json:"order"`
	Role               string `json:"role"`
	SourceKind         string `json:"source_kind"`
	SourceID           string `json:"source_id"`
	SourceDisplayName  string `json:"source_display_name"`
	NodeID             string `json:"node_id"`
	NodeDisplayName    string `json:"node_display_name"`
	ProviderID         string `json:"provider_id"`
	GatewayID          string `json:"gateway_id"`
	GatewayDisplayName string `json:"gateway_display_name"`
	ObservedIP         string `json:"observed_ip"`
	CountryCode        string `json:"country_code"`
	Region             string `json:"region"`
	City               string `json:"city"`
	Status             string `json:"status"`
	DelayMs            uint32 `json:"delay_ms"`
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
	data := proxyUsageData(row.RawJSON)
	return accountProxyUsageResponse{
		ID:              row.ID,
		AccountID:       row.AccountID,
		JobID:           row.JobID,
		N8NExecutionID:  row.N8NExecutionID,
		Purpose:         row.Purpose,
		ProxyURLHash:    row.ProxyURLHash,
		SessionIDHash:   row.SessionIDHash,
		ExitIP:          row.ExitIP,
		CountryCode:     row.CountryCode,
		Region:          row.Region,
		City:            row.City,
		IPFraudCheck:    riskCheckMap(dataMap(data, "ip_fraud_check")),
		EdgeAccessCheck: riskCheckMap(dataMap(data, "edge_access_check")),
		TargetReachable: dataBoolDefault(data, "target_connectivity_reachable", row.Accepted),
		AttemptIndex:    row.AttemptIndex,
		Accepted:        row.Accepted,
		ErrorMessage:    row.ErrorMessage,
		CreatedAt:       row.CreatedAt,
		Chain:           proxyUsageChain(data),
	}
}

func proxyUsageData(raw string) map[string]any {
	var data map[string]any
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &data) != nil {
		return nil
	}
	return data
}

func proxyUsageChain(data map[string]any) proxyUsageChainResponse {
	if chain, ok := data["chain"].(map[string]any); ok {
		return proxyUsageChainResponse{ChainID: dataString(chain, "chain_id"), Hops: proxyUsageChainHops(dataSlice(chain, "hops"))}
	}
	return proxyUsageChainFromPlan(dataMap(data, "chain_plan"))
}

func proxyUsageChainFromPlan(plan map[string]any) proxyUsageChainResponse {
	return proxyUsageChainResponse{ChainID: dataString(plan, "chain_id"), Hops: proxyUsageChainHops(dataSlice(plan, "hops"))}
}

func proxyUsageChainHops(values []any) []proxyUsageChainHopResponse {
	out := make([]proxyUsageChainHopResponse, 0, len(values))
	for _, value := range values {
		hop, _ := value.(map[string]any)
		if len(hop) == 0 {
			continue
		}
		out = append(out, proxyUsageChainHopResponse{
			HopID:              dataString(hop, "hop_id"),
			Order:              dataUint32(hop, "order"),
			Role:               dataString(hop, "role"),
			SourceKind:         dataString(hop, "source_kind"),
			SourceID:           dataString(hop, "source_id"),
			SourceDisplayName:  dataString(hop, "source_display_name"),
			NodeID:             dataString(hop, "node_id"),
			NodeDisplayName:    dataString(hop, "node_display_name"),
			ProviderID:         dataString(hop, "provider_id"),
			GatewayID:          dataString(hop, "gateway_id"),
			GatewayDisplayName: dataString(hop, "gateway_display_name"),
			ObservedIP:         dataString(hop, "observed_ip"),
			CountryCode:        dataString(hop, "country_code"),
			Region:             dataString(hop, "region"),
			City:               dataString(hop, "city"),
			Status:             dataString(hop, "status"),
			DelayMs:            dataUint32(hop, "delay_ms"),
		})
	}
	return out
}

func dataMap(data map[string]any, key string) map[string]any {
	if data == nil {
		return nil
	}
	value, _ := data[key].(map[string]any)
	return value
}

func riskCheckMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return value
	}
	if _, ok := value["risk_score"]; ok {
		return value
	}
	if strings.Contains(strings.ToUpper(dataString(value, "risk_level")), "LOW") {
		out := make(map[string]any, len(value)+1)
		for key, item := range value {
			out[key] = item
		}
		out["risk_score"] = 0
		return out
	}
	return value
}

func dataSlice(data map[string]any, key string) []any {
	if data == nil {
		return nil
	}
	value, _ := data[key].([]any)
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

func dataUint32(data map[string]any, key string) uint32 {
	value := dataFloat(data, key)
	if value <= 0 {
		return 0
	}
	return uint32(value)
}

func dataFloat(data map[string]any, key string) float64 {
	if data == nil {
		return 0
	}
	switch value := data[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case uint32:
		return float64(value)
	case string:
		out, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return out
	default:
		return 0
	}
}

func dataBool(data map[string]any, key string) bool {
	if data == nil {
		return false
	}
	switch value := data[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func dataBoolDefault(data map[string]any, key string, fallback bool) bool {
	if data == nil {
		return fallback
	}
	if _, ok := data[key]; !ok {
		return fallback
	}
	return dataBool(data, key)
}
