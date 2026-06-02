package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/byte-v-forge/common-lib/accountmodel"
	"github.com/byte-v-forge/common-lib/accountstream"
	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/grpchealth"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/pagex"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gpt_account/db"
	"gpt_account/pb"
)

const (
	codexPhoneStatusConfirmed      = "CONFIRMED"
	codexPhoneStatusOAuthNeedPhone = "OAUTH_NEED_PHONE"
)

type gptAccountServer struct {
	pb.UnimplementedGPTAccountServiceServer
	db            *gorm.DB
	state         *accountStateStore
	hot           hotstream.Publisher
	accountStream *accountstream.Publisher
}

func (s *gptAccountServer) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	out, err := s.createGPTAccount(ctx, req.GetAccount(), req.GetCredential())
	if err != nil {
		return nil, err
	}
	return &pb.CreateAccountResponse{Account: out}, nil
}

func (s *gptAccountServer) GetAccount(ctx context.Context, req *pb.GetAccountRequest) (*pb.GetAccountResponse, error) {
	out, err := s.getGPTAccount(ctx, req.GetAccountId())
	if err != nil {
		return nil, err
	}
	return &pb.GetAccountResponse{Account: out}, nil
}

func (s *gptAccountServer) GetAccountCredential(ctx context.Context, req *pb.GetAccountCredentialRequest) (*pb.GetAccountCredentialResponse, error) {
	account, err := s.findAccount(ctx, req.GetAccountId())
	if err != nil {
		return nil, err
	}
	return &pb.GetAccountCredentialResponse{AccountId: account.ID, Credential: &pb.AccountCredential{Password: account.Password}}, nil
}

func (s *gptAccountServer) UpdateAccount(ctx context.Context, req *pb.UpdateAccountRequest) (*pb.UpdateAccountResponse, error) {
	out, err := s.updateGPTAccount(ctx, req.GetAccount(), req.GetCredential())
	if err != nil {
		return nil, err
	}
	return &pb.UpdateAccountResponse{Account: out}, nil
}

func (s *gptAccountServer) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	if err := s.deleteGPTAccount(ctx, req.GetAccountId()); err != nil {
		return nil, err
	}
	return &pb.DeleteAccountResponse{Ack: true}, nil
}

func (s *gptAccountServer) ListAccounts(ctx context.Context, req *pb.ListAccountsRequest) (*pb.ListAccountsResponse, error) {
	return s.listGPTAccounts(ctx, req)
}

func (s *gptAccountServer) UpsertGPTEmailAllocation(ctx context.Context, req *pb.UpsertGPTEmailAllocationRequest) (*pb.UpsertGPTEmailAllocationResponse, error) {
	row, err := buildGPTEmailAllocation(req.GetAllocation())
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing db.GPTEmailAllocation
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", row.Email).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return tx.Create(row).Error
		}

		updates := map[string]any{
			"primary_email": row.PrimaryEmail,
			"is_primary":    row.IsPrimary,
		}
		if row.Status != "" && canRefreshAllocationStatus(existing.Status, row.Status) {
			updates["status"] = row.Status
			updates["last_error"] = row.LastError
			if row.AssignedAccountID != "" {
				updates["assigned_account_id"] = row.AssignedAccountID
			}
		}
		if row.Splittable {
			updates["splittable"] = true
		}
		if row.LastError != "" {
			updates["last_error"] = row.LastError
		}
		if err := tx.Model(&db.GPTEmailAllocation{}).Where("email = ?", row.Email).Updates(updates).Error; err != nil {
			return err
		}
		return refreshPrimaryRegisteredState(tx, row.PrimaryEmail)
	})
	if err != nil {
		return nil, err
	}

	allocation, err := s.findGPTEmailAllocation(ctx, row.Email)
	if err != nil {
		return nil, err
	}
	out := gptEmailAllocationToProto(allocation)
	s.publishAllocationHotStream(ctx, out)
	return &pb.UpsertGPTEmailAllocationResponse{Allocation: out}, nil
}

func (s *gptAccountServer) ListGPTEmailAllocations(ctx context.Context, req *pb.ListGPTEmailAllocationsRequest) (*pb.ListGPTEmailAllocationsResponse, error) {
	limit := pagex.NormalizePageLimit(int(req.GetLimit()))
	cursor, err := decodeEmailAllocationCursor(req.GetCursor())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	rows, hasMore, err := s.emailAllocationPageRows(ctx, req, cursor, limit)
	if err != nil {
		return nil, err
	}
	resp := &pb.ListGPTEmailAllocationsResponse{Allocations: make([]*pb.GPTEmailAllocation, 0, len(rows))}
	for i := range rows {
		resp.Allocations = append(resp.Allocations, gptEmailAllocationToProto(&rows[i]))
	}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		resp.NextCursor = encodeEmailAllocationCursor(last.UpdatedAt, last.Email)
	}
	return resp, nil
}

func (s *gptAccountServer) emailAllocationPageRows(ctx context.Context, req *pb.ListGPTEmailAllocationsRequest, cursor emailAllocationCursor, limit int) ([]db.GPTEmailAllocation, bool, error) {
	query := s.db.WithContext(ctx).Order("updated_at ASC").Order("email ASC").Limit(pagex.KeysetLookaheadLimit(limit))
	if statusFilter := strings.TrimSpace(req.GetStatus()); statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	if email := emailx.Normalize(req.GetEmail()); email != "" {
		query = query.Where("email = ?", email)
	}
	if primaryEmail := emailx.Normalize(req.GetPrimaryEmail()); primaryEmail != "" {
		query = query.Where("primary_email = ?", primaryEmail)
	}
	if req.IsPrimary != nil {
		query = query.Where("is_primary = ?", req.GetIsPrimary())
	}
	if req.GetSplittableOnly() {
		query = query.Where("is_primary = ? AND splittable = ?", true, true)
	}
	if cursor.Valid() {
		query = query.Where("(updated_at, email) > (?, ?)", cursor.UpdatedAt, cursor.Email)
	}

	var rows []db.GPTEmailAllocation
	if err := query.Find(&rows).Error; err != nil {
		return nil, false, err
	}
	rows, hasMore := pagex.TrimLimit(rows, limit)
	return rows, hasMore, nil
}

type emailAllocationCursor struct {
	UpdatedAt int64  `json:"updated_at"`
	Email     string `json:"email"`
}

func (c emailAllocationCursor) Valid() bool {
	return c.UpdatedAt >= 0 && emailx.Normalize(c.Email) != ""
}

func encodeEmailAllocationCursor(updatedAt int64, email string) string {
	email = emailx.Normalize(email)
	if updatedAt < 0 || email == "" {
		return ""
	}
	data, err := json.Marshal(emailAllocationCursor{UpdatedAt: updatedAt, Email: email})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeEmailAllocationCursor(value string) (emailAllocationCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return emailAllocationCursor{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return emailAllocationCursor{}, fmt.Errorf("invalid email allocation cursor")
	}
	var cursor emailAllocationCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return emailAllocationCursor{}, fmt.Errorf("invalid email allocation cursor")
	}
	cursor.Email = emailx.Normalize(cursor.Email)
	if !cursor.Valid() {
		return emailAllocationCursor{}, fmt.Errorf("invalid email allocation cursor")
	}
	return cursor, nil
}

func (s *gptAccountServer) ClaimGPTEmailAllocation(ctx context.Context, req *pb.ClaimGPTEmailAllocationRequest) (*pb.ClaimGPTEmailAllocationResponse, error) {
	email := emailx.Normalize(req.GetEmail())
	accountID, accountErr := normalizeAccountID(req.GetAccountId())
	nextStatus := strings.TrimSpace(req.GetStatus())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if accountErr != nil {
		return nil, accountErr
	}
	if nextStatus == "" {
		nextStatus = gptplugin.EmailStatusAssigned
	}

	var claimed bool
	var row db.GPTEmailAllocation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", email)
		if expected := strings.TrimSpace(req.GetExpectedStatus()); expected != "" {
			query = query.Where("status = ?", expected)
		}
		result := query.Find(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if req.GetRequirePrimarySplittable() {
			var primary db.GPTEmailAllocation
			primaryQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("email = ? AND is_primary = ? AND splittable = ?", row.PrimaryEmail, true, true)
			if expectedPrimaryStatus := strings.TrimSpace(req.GetExpectedPrimaryStatus()); expectedPrimaryStatus != "" {
				primaryQuery = primaryQuery.Where("status = ?", expectedPrimaryStatus)
			}
			result := primaryQuery.Find(&primary)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
		}
		if err := tx.Model(&db.GPTEmailAllocation{}).Where("email = ?", row.Email).Updates(map[string]any{
			"status":              nextStatus,
			"assigned_account_id": accountID,
			"last_error":          "",
		}).Error; err != nil {
			return err
		}
		row.Status = nextStatus
		row.AssignedAccountID = accountID
		row.LastError = ""
		claimed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !claimed {
		return &pb.ClaimGPTEmailAllocationResponse{Claimed: false}, nil
	}
	allocation, err := s.findGPTEmailAllocation(ctx, row.Email)
	if err != nil {
		return nil, err
	}
	out := gptEmailAllocationToProto(allocation)
	s.publishAllocationHotStream(ctx, out)
	return &pb.ClaimGPTEmailAllocationResponse{Claimed: true, Allocation: out}, nil
}

func (s *gptAccountServer) CreateGPTEmailAliasAllocation(ctx context.Context, req *pb.CreateGPTEmailAliasAllocationRequest) (*pb.CreateGPTEmailAliasAllocationResponse, error) {
	primaryEmail := emailx.Normalize(req.GetPrimaryEmail())
	accountID, accountErr := normalizeAccountID(req.GetAccountId())
	if primaryEmail == "" {
		return nil, status.Error(codes.InvalidArgument, "primary_email is required")
	}
	if accountErr != nil {
		return nil, accountErr
	}

	var created *db.GPTEmailAllocation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var primary db.GPTEmailAllocation
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("email = ? AND is_primary = ? AND status = ? AND splittable = ?", primaryEmail, true, gptplugin.EmailStatusRegistered, true).
			Find(&primary)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		for i := 0; i < 20; i++ {
			alias, err := db.RandomAliasEmail(primary.Email, 6)
			if err != nil {
				return err
			}
			if alias == "" {
				return fmt.Errorf("invalid primary email: %s", emailx.Redact(primary.Email))
			}
			row := &db.GPTEmailAllocation{
				Email:             alias,
				PrimaryEmail:      primary.Email,
				IsPrimary:         false,
				Status:            gptplugin.EmailStatusAssigned,
				Splittable:        false,
				AssignedAccountID: accountID,
				LastError:         "",
			}
			err = tx.Create(row).Error
			if err == nil {
				created = row
				return nil
			}
			if !isUniqueViolation(err) {
				return err
			}
		}
		return fmt.Errorf("failed to create unique alias for %s", emailx.Redact(primary.Email))
	})
	if err != nil {
		return nil, err
	}
	if created == nil {
		return &pb.CreateGPTEmailAliasAllocationResponse{Created: false}, nil
	}
	allocation, err := s.findGPTEmailAllocation(ctx, created.Email)
	if err != nil {
		return nil, err
	}
	out := gptEmailAllocationToProto(allocation)
	s.publishAllocationHotStream(ctx, out)
	return &pb.CreateGPTEmailAliasAllocationResponse{Created: true, Allocation: out}, nil
}

func (s *gptAccountServer) MarkGPTEmailAllocationStatus(ctx context.Context, req *pb.MarkGPTEmailAllocationStatusRequest) (*pb.MarkGPTEmailAllocationStatusResponse, error) {
	email := emailx.Normalize(req.GetEmail())
	nextStatus := strings.TrimSpace(req.GetStatus())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if nextStatus == "" {
		return nil, status.Error(codes.InvalidArgument, "status is required")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.GPTEmailAllocation
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", email).Find(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		updates := map[string]any{
			"status":     nextStatus,
			"last_error": strings.TrimSpace(req.GetLastError()),
		}
		if nextStatus == gptplugin.EmailStatusRegistered && row.IsPrimary {
			updates["splittable"] = true
		}
		if nextStatus == gptplugin.EmailStatusUserAlreadyExists || nextStatus == gptplugin.EmailStatusBlocked {
			updates["splittable"] = false
		}
		if err := tx.Model(&db.GPTEmailAllocation{}).Where("email = ?", row.Email).Updates(updates).Error; err != nil {
			return err
		}
		if nextStatus == gptplugin.EmailStatusRegistered {
			if err := refreshPrimaryRegisteredState(tx, row.PrimaryEmail); err != nil {
				return err
			}
		}
		if nextStatus == gptplugin.EmailStatusUserAlreadyExists {
			primaryEmail := row.PrimaryEmail
			if primaryEmail == "" {
				primaryEmail = row.Email
			}
			blockUpdate := map[string]any{
				"status":     gptplugin.EmailStatusBlocked,
				"splittable": false,
				"last_error": strings.TrimSpace(req.GetLastError()),
			}
			if err := tx.Model(&db.GPTEmailAllocation{}).
				Where("email = ? AND is_primary = ? AND status <> ?", primaryEmail, true, gptplugin.EmailStatusUserAlreadyExists).
				Updates(blockUpdate).Error; err != nil {
				return err
			}
			if err := tx.Model(&db.GPTEmailAllocation{}).
				Where("primary_email = ? AND is_primary = ? AND status = ?", primaryEmail, false, gptplugin.EmailStatusAvailable).
				Updates(map[string]any{"status": gptplugin.EmailStatusBlocked, "last_error": strings.TrimSpace(req.GetLastError())}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	allocation, err := s.findGPTEmailAllocation(ctx, email)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return &pb.MarkGPTEmailAllocationStatusResponse{}, nil
		}
		return nil, err
	}
	out := gptEmailAllocationToProto(allocation)
	s.publishAllocationHotStream(ctx, out)
	return &pb.MarkGPTEmailAllocationStatusResponse{Allocation: out}, nil
}

func (s *gptAccountServer) buildAccount(input *pb.Account, credential *pb.AccountCredential) (*db.Account, error) {
	if input == nil {
		input = &pb.Account{}
	}

	account := &db.Account{
		ID:       gptAccountID(input),
		Email:    gptAccountEmail(input),
		Password: strings.TrimSpace(credential.GetPassword()),
	}

	if account.ID == "" {
		account.ID = gofakeit.UUID()
	}
	accountID, err := normalizeAccountID(account.ID)
	if err != nil {
		return nil, err
	}
	account.ID = accountID
	account.Email = emailx.Normalize(account.Email)
	if account.Password == "" {
		account.Password = gofakeit.Password(true, true, true, true, false, 12)
	}

	return account, nil
}

func buildGPTEmailAllocation(input *pb.GPTEmailAllocation) (*db.GPTEmailAllocation, error) {
	if input == nil {
		return nil, status.Error(codes.InvalidArgument, "allocation is required")
	}
	email := emailx.Normalize(input.GetEmail())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	primaryEmail := emailx.Normalize(input.GetPrimaryEmail())
	if primaryEmail == "" {
		primaryEmail = emailx.CanonicalPlusAlias(email)
	}
	isPrimary := input.GetIsPrimary()
	if primaryEmail == email {
		isPrimary = true
	}
	assignedAccountID, err := normalizeOptionalAccountID(input.GetAssignedAccountId(), "assigned_account_id")
	if err != nil {
		return nil, err
	}
	row := &db.GPTEmailAllocation{
		Email:             email,
		PrimaryEmail:      primaryEmail,
		IsPrimary:         isPrimary,
		Status:            strings.TrimSpace(input.GetStatus()),
		Splittable:        input.GetSplittable(),
		AssignedAccountID: assignedAccountID,
		LastError:         strings.TrimSpace(input.GetLastError()),
	}
	if row.Status == "" {
		row.Status = gptplugin.EmailStatusAvailable
	}
	return row, nil
}

func assignAccountEmailAllocation(tx *gorm.DB, accountID string, email string) error {
	email = emailx.Normalize(email)
	accountID = strings.TrimSpace(accountID)
	if email == "" || accountID == "" {
		return nil
	}

	var existing db.GPTEmailAllocation
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", email).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return tx.Create(&db.GPTEmailAllocation{
			Email:             email,
			PrimaryEmail:      emailx.CanonicalPlusAlias(email),
			IsPrimary:         emailx.CanonicalPlusAlias(email) == email,
			Status:            gptplugin.EmailStatusAssigned,
			Splittable:        false,
			AssignedAccountID: accountID,
			LastError:         "",
		}).Error
	}
	if !canRefreshAllocationStatus(existing.Status, gptplugin.EmailStatusAssigned) && existing.AssignedAccountID != accountID {
		return nil
	}
	return tx.Model(&db.GPTEmailAllocation{}).Where("email = ?", email).Updates(map[string]any{
		"status":              gptplugin.EmailStatusAssigned,
		"assigned_account_id": accountID,
		"last_error":          "",
	}).Error
}

func refreshPrimaryRegisteredState(tx *gorm.DB, primaryEmail string) error {
	primaryEmail = emailx.Normalize(primaryEmail)
	if primaryEmail == "" {
		return nil
	}
	return tx.Model(&db.GPTEmailAllocation{}).
		Where("email = ? AND is_primary = ? AND status NOT IN ?", primaryEmail, true, []string{gptplugin.EmailStatusUserAlreadyExists, gptplugin.EmailStatusBlocked}).
		Where("EXISTS (SELECT 1 FROM gpt_email_allocations AS child WHERE child.primary_email = ? AND child.status = ?)", primaryEmail, gptplugin.EmailStatusRegistered).
		Updates(map[string]any{
			"status":     gptplugin.EmailStatusRegistered,
			"splittable": true,
		}).Error
}

func (s *gptAccountServer) findAccount(ctx context.Context, accountID string) (*db.Account, error) {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return nil, err
	}

	var account db.Account
	err = s.db.WithContext(ctx).First(&account, "id = ?", accountID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *gptAccountServer) findGPTEmailAllocation(ctx context.Context, email string) (*db.GPTEmailAllocation, error) {
	email = emailx.Normalize(email)
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	var row db.GPTEmailAllocation
	err := s.db.WithContext(ctx).First(&row, "email = ?", email).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, status.Error(codes.NotFound, "gpt email allocation not found")
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func normalizeAccountID(value string) (string, error) {
	accountID, err := gptAccountDescriptor.NormalizeID(value, "account_id")
	if err != nil {
		return "", status.Error(codes.InvalidArgument, err.Error())
	}
	return accountID, nil
}

func normalizeOptionalAccountID(value string, field string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	accountID, err := gptAccountDescriptor.NormalizeID(value, field)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, err.Error())
	}
	return accountID, nil
}

func updateMap(account *pb.Account, credential *pb.AccountCredential) map[string]interface{} {
	updates := map[string]interface{}{}
	if account == nil {
		return updates
	}

	if value := gptAccountEmail(account); value != "" {
		updates["email"] = value
	}
	if value := strings.TrimSpace(credential.GetPassword()); value != "" {
		updates["password"] = value
	}
	return updates
}

func (s *gptAccountServer) accountToProto(ctx context.Context, account *db.Account) (*pb.Account, error) {
	if account == nil {
		return nil, nil
	}
	out := &pb.Account{
		Account:             newGptAccountRecord(account.ID, account.Email, account.CreatedAt, account.UpdatedAt),
		PrimaryMailboxEmail: emailx.CanonicalPlusAlias(account.Email),
	}
	if err := s.state.apply(ctx, out); err != nil {
		return nil, err
	}
	accountmodel.SetCredentialState(ensureGptAccountRecord(out), accountmodel.CredentialKindPassword, strings.TrimSpace(account.Password) != "", accountmodel.CredentialStatusConfigured, time.Time{}, time.Time{})
	out.Account = gptAccountProjection(out)
	return out, nil
}

func gptEmailAllocationToProto(row *db.GPTEmailAllocation) *pb.GPTEmailAllocation {
	if row == nil {
		return nil
	}
	return &pb.GPTEmailAllocation{
		Email:             row.Email,
		PrimaryEmail:      row.PrimaryEmail,
		IsPrimary:         row.IsPrimary,
		Status:            row.Status,
		Splittable:        row.Splittable,
		AssignedAccountId: row.AssignedAccountID,
		LastError:         row.LastError,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func normalizeTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}

func normalizeCodexPhoneStatus(value string) string {
	status := strings.ToUpper(strings.TrimSpace(value))
	status = strings.NewReplacer(" ", "_", "-", "_").Replace(status)
	switch status {
	case codexPhoneStatusConfirmed, codexPhoneStatusOAuthNeedPhone:
		return status
	default:
		return ""
	}
}

func canRefreshAllocationStatus(current string, incoming string) bool {
	current = strings.TrimSpace(current)
	incoming = strings.TrimSpace(incoming)
	switch current {
	case "", gptplugin.EmailStatusAvailable, gptplugin.EmailStatusOAuthPending, gptplugin.EmailStatusAuthFailed:
		return true
	case gptplugin.EmailStatusRegistered:
		return incoming == gptplugin.EmailStatusRegistered || incoming == gptplugin.EmailStatusUserAlreadyExists || incoming == gptplugin.EmailStatusBlocked
	case gptplugin.EmailStatusAssigned:
		return incoming == gptplugin.EmailStatusUserAlreadyExists || incoming == gptplugin.EmailStatusBlocked
	case gptplugin.EmailStatusUserAlreadyExists, gptplugin.EmailStatusBlocked:
		return false
	default:
		return incoming != gptplugin.EmailStatusAvailable && incoming != gptplugin.EmailStatusOAuthPending && incoming != gptplugin.EmailStatusAuthFailed
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "duplicate key") || strings.Contains(text, "unique constraint")
}

func main() {
	log.Println("Initializing GPT account service...")
	gofakeit.Seed(time.Now().UnixNano())
	ctx := context.Background()
	database := db.InitDB()
	stateStore, closeStateStore, err := newAccountStateStore(ctx)
	if err != nil {
		log.Fatalf("failed to initialize account state cache: %v", err)
	}
	defer closeStateStore()
	platformNATSURL := envx.StringDefault("PLATFORM_NATS_URL", "")
	hot, closeHot, err := newAccountHotStream(ctx, platformNATSURL)
	if err != nil {
		log.Fatalf("failed to initialize account hotstream: %v", err)
	}
	defer closeHot()

	listenAddr := envx.StringDefault("LISTEN_ADDR", ":50051")
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterGPTAccountServiceServer(grpcServer, &gptAccountServer{
		db:            database,
		state:         stateStore,
		hot:           hot,
		accountStream: accountstream.NewPublisher(accountstream.Config{Publisher: hot, Descriptor: gptAccountDescriptor}),
	})
	grpchealth.RegisterServing(grpcServer)

	log.Printf("GPT account gRPC server listening on %s", listenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
