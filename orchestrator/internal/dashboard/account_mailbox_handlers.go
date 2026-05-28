package dashboard

import (
	"context"
	"net/http"
	"time"

	"github.com/byte-v-forge/common-lib/envx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"orchestrator/pb"
)

func (s *server) handleAccountMailboxInbox(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req mailboxInboxRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.LimitPerMailbox <= 0 {
		req.LimitPerMailbox = 10
	}
	if req.LimitPerMailbox > 100 {
		req.LimitPerMailbox = 100
	}

	timeout := envx.Int("ACCOUNT_MAILBOX_INBOX_TIMEOUT_SECONDS", envx.Int("MAILBOX_INBOX_TIMEOUT_SECONDS", 180))
	if timeout < 30 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	resp, err := s.accountWorkflowClient.FetchAccountMailbox(ctx, &pb.FetchAccountMailboxRequest{
		AccountId:       accountID,
		LimitPerMailbox: req.LimitPerMailbox,
	})
	if err != nil {
		if status.Code(err) == codes.DeadlineExceeded {
			writeError(w, http.StatusGatewayTimeout, err)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
