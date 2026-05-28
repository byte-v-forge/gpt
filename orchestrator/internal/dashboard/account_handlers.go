package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/byte-v-forge/common-lib/httpx"

	"orchestrator/internal/chatgptauth"
	"orchestrator/pb"
)

type createAccountRequest struct {
	Email          string `json:"email"`
	EmailDomain    string `json:"email_domain"`
	EmailLocalPart string `json:"email_local_part"`
	Domain         string `json:"domain"`
	LocalPart      string `json:"local_part"`
	Password       string `json:"password"`
	EmailStrategy  string `json:"email_strategy"`
	CountryCode    string `json:"country_code"`
	Region         string `json:"region"`
}

type updateAccountRequest struct {
	SessionToken      string  `json:"session_token"`
	AccessToken       string  `json:"access_token"`
	ActivationChannel *string `json:"activation_channel"`
}

func (s *server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := int32(httpx.QueryInt(r, "limit", 100))
		resp, err := s.accountClient.ListAccounts(r.Context(), &pb.ListAccountsRequest{
			Status: r.URL.Query().Get("status"),
			Limit:  limit,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		accounts := resp.GetAccounts()
		if accounts == nil {
			accounts = []*pb.Account{}
		}
		writeJSON(w, http.StatusOK, accounts)
	case http.MethodPost:
		var req createAccountRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		email := createAccountEmail(req)
		emailStrategy, err := accountEmailStrategy(req.EmailStrategy, email)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		accountID := randomID()
		resp, err := s.accountWorkflowClient.CreateGPTAccount(r.Context(), &pb.CreateGPTAccountRequest{
			AccountId:     accountID,
			Email:         email,
			Password:      req.Password,
			EmailStrategy: emailStrategy,
			CountryCode:   req.CountryCode,
			Region:        req.Region,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if resp.GetErrorMessage() != "" {
			writeError(w, http.StatusBadGateway, errors.New(resp.GetErrorMessage()))
			return
		}
		writeJSON(w, http.StatusCreated, resp.GetAccount())
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func createAccountEmail(req createAccountRequest) string {
	email := normalizeEmailAddress(req.Email)
	if email != "" {
		return email
	}
	domain := normalizeEmailDomain(firstNonEmpty(req.EmailDomain, req.Domain))
	if domain == "" {
		return ""
	}
	localPart := normalizeEmailLocalPart(firstNonEmpty(req.EmailLocalPart, req.LocalPart))
	if localPart == "" {
		localPart = randomFakeEmailLocalPart()
	}
	return localPart + "@" + domain
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func randomFakeEmailLocalPart() string {
	firstName := normalizeEmailLocalPart(gofakeit.FirstName())
	lastName := normalizeEmailLocalPart(gofakeit.LastName())
	suffix := fmt.Sprintf("%d", gofakeit.Number(100, 9999))
	localPart := strings.Join(compactEmailLocalParts(firstName, lastName, suffix), "")
	if localPart != "" {
		return localPart
	}
	return strings.ToLower(randomID())
}

func compactEmailLocalParts(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeEmailLocalPart(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizeEmailAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value)
}

func normalizeEmailDomain(value string) string {
	value = strings.Trim(strings.TrimSpace(strings.ToLower(value)), "@. ")
	return value
}

func normalizeEmailLocalPart(value string) string {
	value = strings.TrimSpace(strings.Split(value, "@")[0])
	var builder strings.Builder
	lastDot := false
	for _, char := range strings.ToLower(value) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
			lastDot = false
			continue
		}
		if char == '.' || char == ' ' {
			if !lastDot {
				builder.WriteByte('.')
				lastDot = true
			}
		}
	}
	return strings.Trim(builder.String(), ".-_")
}

func accountEmailStrategy(value string, email string) (pb.AccountEmailStrategy, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_EXPLICIT.String():
		if strings.TrimSpace(email) == "" {
			return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED, fmt.Errorf("%s strategy requires email", value)
		}
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_EXPLICIT, nil
	case pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_PRIMARY.String():
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_PRIMARY, nil
	case pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS.String():
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS, nil
	case "":
		if strings.TrimSpace(email) != "" {
			return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_EXPLICIT, nil
		}
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS, nil
	default:
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED, fmt.Errorf("unsupported email strategy: %s", value)
	}
}

func (s *server) handleAccount(w http.ResponseWriter, r *http.Request) {
	accountPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/accounts/"), "/")
	parts := strings.Split(accountPath, "/")
	accountID := parts[0]
	if accountID == "" {
		writeError(w, http.StatusBadRequest, errors.New("account_id is required"))
		return
	}
	if len(parts) > 1 {
		if len(parts) == 2 && parts[1] == "access-token" {
			s.handleAccountAccessToken(w, r, accountID)
			return
		}
		if len(parts) == 2 && parts[1] == "auth" {
			s.handleAccountAuth(w, r, accountID)
			return
		}
		if len(parts) == 2 && parts[1] == "checkout-link" {
			s.handleAccountCheckoutLink(w, r, accountID)
			return
		}
		if parts[1] == "fingerprint" {
			action := ""
			if len(parts) == 3 {
				action = parts[2]
			}
			if len(parts) <= 3 {
				s.handleAccountFingerprint(w, r, accountID, action)
				return
			}
		}
		if len(parts) == 2 && parts[1] == "proxy-usages" {
			s.handleAccountProxyUsages(w, r, accountID)
			return
		}
		if len(parts) == 3 && parts[1] == "mailbox" && parts[2] == "inbox" {
			s.handleAccountMailboxInbox(w, r, accountID)
			return
		}
		writeError(w, http.StatusNotFound, errors.New("account endpoint not found"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		resp, err := s.accountClient.GetAccount(r.Context(), &pb.GetAccountRequest{AccountId: accountID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, resp.GetAccount())
	case http.MethodPatch, http.MethodPut:
		var req updateAccountRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		sessionToken, accessToken := normalizeAccountAuthInput(req.SessionToken, req.AccessToken)
		if sessionToken == "" && accessToken == "" && req.ActivationChannel == nil {
			writeError(w, http.StatusBadRequest, errors.New("session_token, access_token, or activation_channel is required"))
			return
		}
		if err := s.saveAccountAuth(r.Context(), accountID, sessionToken, accessToken); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		account := &pb.Account{
			AccountId: accountID,
		}
		if sessionToken != "" || accessToken != "" {
			account.Status = "REGISTERED"
			account.ErrorMessage = ""
		}
		if req.ActivationChannel != nil {
			activationChannel := strings.TrimSpace(*req.ActivationChannel)
			account.ActivationChannel = &activationChannel
		}
		if req.ActivationChannel != nil || account.GetStatus() != "" {
			resp, err := s.accountClient.UpdateAccount(r.Context(), &pb.UpdateAccountRequest{Account: account})
			if err != nil {
				writeError(w, http.StatusBadGateway, err)
				return
			}
			writeJSON(w, http.StatusOK, resp.GetAccount())
			return
		}
		resp, err := s.accountClient.GetAccount(r.Context(), &pb.GetAccountRequest{AccountId: accountID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, resp.GetAccount())
	case http.MethodDelete:
		resp, err := s.accountClient.DeleteAccount(r.Context(), &pb.DeleteAccountRequest{AccountId: accountID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if s.runtimeSecrets != nil {
			_ = s.runtimeSecrets.Delete(r.Context(), chatgptauth.AccountAuthSecretKey(accountID))
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
