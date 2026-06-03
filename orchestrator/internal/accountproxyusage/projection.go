package accountproxyusage

import (
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"orchestrator/pb"
)

func ListResponse(rows []Usage) *pb.ListAccountProxyUsagesResponse {
	out := make([]*pb.AccountProxyUsage, 0, len(rows))
	for _, row := range rows {
		out = append(out, Response(row))
	}
	return &pb.ListAccountProxyUsagesResponse{Usages: out}
}

func Response(row Usage) *pb.AccountProxyUsage {
	data := preflightData(row.RawJSON)
	return &pb.AccountProxyUsage{
		Id:              row.ID,
		AccountId:       row.AccountID,
		JobId:           row.JobID,
		N8NExecutionId:  row.N8NExecutionID,
		Purpose:         row.Purpose,
		ProxyUrlHash:    row.ProxyURLHash,
		SessionIdHash:   row.SessionIDHash,
		ExitIp:          row.ExitIP,
		CountryCode:     row.CountryCode,
		Region:          row.Region,
		City:            row.City,
		IpFraudCheck:    data.GetIpFraudCheck(),
		EdgeAccessCheck: data.GetEdgeAccessCheck(),
		TargetReachable: targetReachable(row, data),
		AttemptIndex:    row.AttemptIndex,
		Accepted:        row.Accepted,
		ErrorMessage:    row.ErrorMessage,
		CreatedAt:       row.CreatedAt,
		Route:           data.GetRoutePlan(),
	}
}

func preflightData(raw string) *pb.N8NDynamicProxyPreflightData {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := &pb.N8NDynamicProxyPreflightData{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(raw), out); err != nil {
		return nil
	}
	return out
}

func targetReachable(row Usage, data *pb.N8NDynamicProxyPreflightData) bool {
	if data == nil {
		return row.Accepted
	}
	if data.GetTargetConnectivityCheck() != nil {
		return data.GetTargetConnectivityCheck().GetReachable()
	}
	if data.GetTargetConnectivityEnabled() {
		return data.GetTargetConnectivityReachable()
	}
	return row.Accepted
}
