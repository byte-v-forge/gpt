package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/protobuf/proto"

	"orchestrator/pb"
)

func (s *server) handleGoPayUserAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/gopay/user/"), "/")
	switch action {
	case "check-phone":
		var req pb.GoPayUserCheckPhoneRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserCheckPhone(r.Context(), &req) })
		return
	case "check-balance":
		var req pb.GoPayUserCheckBalanceRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserCheckBalance(r.Context(), &req) })
		return
	case "auth-start":
		var req pb.GoPayUserAuthStartRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserAuthStart(r.Context(), &req) })
		return
	case "auth-complete":
		var req pb.GoPayUserAuthCompleteRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserAuthComplete(r.Context(), &req) })
		return
	case "signup-start":
		var req pb.GoPayUserSignupStartRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserSignupStart(r.Context(), &req) })
		return
	case "signup-complete":
		var req pb.GoPayUserSignupCompleteRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserSignupComplete(r.Context(), &req) })
		return
	case "change-phone-start":
		var req pb.GoPayUserChangePhoneStartRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserChangePhoneStart(r.Context(), &req) })
		return
	case "change-phone-complete":
		var req pb.GoPayUserChangePhoneCompleteRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserChangePhoneComplete(r.Context(), &req) })
		return
	case "change-phone-retry":
		var req pb.GoPayUserChangePhoneRetryRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserChangePhoneRetry(r.Context(), &req) })
		return
	case "create-pin-start":
		var req pb.GoPayUserCreatePinStartRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserCreatePinStart(r.Context(), &req) })
		return
	case "create-pin-complete":
		var req pb.GoPayUserCreatePinCompleteRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserCreatePinComplete(r.Context(), &req) })
		return
	case "clear-state":
		var req pb.GoPayUserClearStateRequest
		serveProtoAction(w, r, &req, func() (proto.Message, error) { return s.gopayAppClient.GoPayUserClearState(r.Context(), &req) })
		return
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown gopay action: %s", action))
	}
}
