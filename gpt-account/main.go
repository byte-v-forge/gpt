package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/grpchealth"
	"github.com/byte-v-forge/common-lib/hotstream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gpt_account/db"
	"gpt_account/pb"
)

const (
	gptEmailStatusAvailable         = "AVAILABLE"
	gptEmailStatusAssigned          = "ASSIGNED"
	gptEmailStatusRegistered        = "REGISTERED"
	gptEmailStatusOAuthPending      = "OAUTH_PENDING"
	gptEmailStatusAuthFailed        = "AUTH_FAILED"
	gptEmailStatusUserAlreadyExists = "USER_ALREADY_EXISTS"
	gptEmailStatusBlocked           = "BLOCKED"
	codexPhoneStatusConfirmed       = "CONFIRMED"
	codexPhoneStatusOAuthNeedPhone  = "OAUTH_NEED_PHONE"
)

type gptAccountServer struct {
	pb.UnimplementedGPTAccountServiceServer
	db    *gorm.DB
	state *accountStateStore
	hot   hotstream.Publisher
}

func (s *gptAccountServer) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	account, err := s.buildAccount(req.GetAccount())
	if err != nil {
		return nil, err
	}
	if account.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(account).Error; err != nil {
			return err
		}
		return assignAccountEmailAllocation(tx, account.ID, account.Email)
	}); err != nil {
		return nil, err
	}
	if err := s.state.saveInitial(ctx, account.ID, req.GetAccount()); err != nil {
		return nil, err
	}

	out, err := s.accountToProto(ctx, account)
	if err != nil {
		return nil, err
	}
	log.Printf("Created account id=%s email=%s", account.ID, emailx.Redact(account.Email))
	s.publishAccountHotStream(ctx, accountUpdatedEvent, out)
	return &pb.CreateAccountResponse{Account: out}, nil
}

func (s *gptAccountServer) GetAccount(ctx context.Context, req *pb.GetAccountRequest) (*pb.GetAccountResponse, error) {
	account, err := s.findAccount(ctx, req.GetAccountId())
	if err != nil {
		return nil, err
	}
	out, err := s.accountToProto(ctx, account)
	if err != nil {
		return nil, err
	}
	return &pb.GetAccountResponse{Account: out}, nil
}

func (s *gptAccountServer) UpdateAccount(ctx context.Context, req *pb.UpdateAccountRequest) (*pb.UpdateAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccount().GetAccountId())
	if accountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	if _, err := s.findAccount(ctx, accountID); err != nil {
		return nil, err
	}

	updates := updateMap(req.GetAccount())
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&db.Account{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := s.state.savePatch(ctx, accountID, req.GetAccount()); err != nil {
		return nil, err
	}

	account, err := s.findAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out, err := s.accountToProto(ctx, account)
	if err != nil {
		return nil, err
	}
	log.Printf("Updated account id=%s status=%s", account.ID, out.GetStatus())
	s.publishAccountHotStream(ctx, accountUpdatedEvent, out)
	return &pb.UpdateAccountResponse{Account: out}, nil
}

func (s *gptAccountServer) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	result := s.db.WithContext(ctx).Delete(&db.Account{}, "id = ?", accountID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	if err := s.state.delete(ctx, accountID); err != nil {
		return nil, err
	}
	s.publishAccountHotStream(ctx, accountDeletedEvent, &pb.Account{AccountId: accountID, UpdatedAt: time.Now().Unix()})
	return &pb.DeleteAccountResponse{Ack: true}, nil
}

func (s *gptAccountServer) ListAccounts(ctx context.Context, req *pb.ListAccountsRequest) (*pb.ListAccountsResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	statusFilter := strings.TrimSpace(req.GetStatus())
	dbLimit := limit
	if statusFilter != "" {
		dbLimit = 500
	}
	query := s.db.WithContext(ctx).Order("created_at DESC").Limit(dbLimit)
	if emailFilter := emailx.Normalize(req.GetEmail()); emailFilter != "" {
		query = query.Where("email = ?", emailFilter)
	}

	var accounts []db.Account
	if err := query.Find(&accounts).Error; err != nil {
		return nil, err
	}

	resp := &pb.ListAccountsResponse{Accounts: make([]*pb.Account, 0, len(accounts))}
	for i := range accounts {
		account, err := s.accountToProto(ctx, &accounts[i])
		if err != nil {
			return nil, err
		}
		if statusFilter != "" && account.GetStatus() != statusFilter {
			continue
		}
		resp.Accounts = append(resp.Accounts, account)
		if len(resp.Accounts) >= limit {
			break
		}
	}
	return resp, nil
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
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := s.db.WithContext(ctx).Order("updated_at ASC").Limit(limit)
	if statusFilter := strings.TrimSpace(req.GetStatus()); statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	if primaryEmail := emailx.Normalize(req.GetPrimaryEmail()); primaryEmail != "" {
		query = query.Where("primary_email = ?", primaryEmail)
	}
	if req.GetSplittableOnly() {
		query = query.Where("is_primary = ? AND splittable = ?", true, true)
	}

	var rows []db.GPTEmailAllocation
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	resp := &pb.ListGPTEmailAllocationsResponse{Allocations: make([]*pb.GPTEmailAllocation, 0, len(rows))}
	for i := range rows {
		resp.Allocations = append(resp.Allocations, gptEmailAllocationToProto(&rows[i]))
	}
	return resp, nil
}

func (s *gptAccountServer) ClaimGPTEmailAllocation(ctx context.Context, req *pb.ClaimGPTEmailAllocationRequest) (*pb.ClaimGPTEmailAllocationResponse, error) {
	email := emailx.Normalize(req.GetEmail())
	accountID := strings.TrimSpace(req.GetAccountId())
	nextStatus := strings.TrimSpace(req.GetStatus())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if accountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if nextStatus == "" {
		nextStatus = gptEmailStatusAssigned
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
	accountID := strings.TrimSpace(req.GetAccountId())
	if primaryEmail == "" {
		return nil, status.Error(codes.InvalidArgument, "primary_email is required")
	}
	if accountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	var created *db.GPTEmailAllocation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var primary db.GPTEmailAllocation
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("email = ? AND is_primary = ? AND status = ? AND splittable = ?", primaryEmail, true, gptEmailStatusRegistered, true).
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
				Status:            gptEmailStatusAssigned,
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
		if nextStatus == gptEmailStatusRegistered && row.IsPrimary {
			updates["splittable"] = true
		}
		if nextStatus == gptEmailStatusUserAlreadyExists || nextStatus == gptEmailStatusBlocked {
			updates["splittable"] = false
		}
		if err := tx.Model(&db.GPTEmailAllocation{}).Where("email = ?", row.Email).Updates(updates).Error; err != nil {
			return err
		}
		if nextStatus == gptEmailStatusRegistered {
			if err := refreshPrimaryRegisteredState(tx, row.PrimaryEmail); err != nil {
				return err
			}
		}
		if nextStatus == gptEmailStatusUserAlreadyExists {
			primaryEmail := row.PrimaryEmail
			if primaryEmail == "" {
				primaryEmail = row.Email
			}
			blockUpdate := map[string]any{
				"status":     gptEmailStatusBlocked,
				"splittable": false,
				"last_error": strings.TrimSpace(req.GetLastError()),
			}
			if err := tx.Model(&db.GPTEmailAllocation{}).
				Where("email = ? AND is_primary = ? AND status <> ?", primaryEmail, true, gptEmailStatusUserAlreadyExists).
				Updates(blockUpdate).Error; err != nil {
				return err
			}
			if err := tx.Model(&db.GPTEmailAllocation{}).
				Where("primary_email = ? AND is_primary = ? AND status = ?", primaryEmail, false, gptEmailStatusAvailable).
				Updates(map[string]any{"status": gptEmailStatusBlocked, "last_error": strings.TrimSpace(req.GetLastError())}).Error; err != nil {
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

func (s *gptAccountServer) buildAccount(input *pb.Account) (*db.Account, error) {
	if input == nil {
		input = &pb.Account{}
	}

	account := &db.Account{
		ID:       strings.TrimSpace(input.GetAccountId()),
		Email:    strings.TrimSpace(input.GetEmail()),
		Password: input.GetPassword(),
	}

	if account.ID == "" {
		account.ID = gofakeit.UUID()
	}
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
	row := &db.GPTEmailAllocation{
		Email:             email,
		PrimaryEmail:      primaryEmail,
		IsPrimary:         isPrimary,
		Status:            strings.TrimSpace(input.GetStatus()),
		Splittable:        input.GetSplittable(),
		AssignedAccountID: strings.TrimSpace(input.GetAssignedAccountId()),
		LastError:         strings.TrimSpace(input.GetLastError()),
	}
	if row.Status == "" {
		row.Status = gptEmailStatusAvailable
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
			Status:            gptEmailStatusAssigned,
			Splittable:        false,
			AssignedAccountID: accountID,
			LastError:         "",
		}).Error
	}
	if !canRefreshAllocationStatus(existing.Status, gptEmailStatusAssigned) && existing.AssignedAccountID != accountID {
		return nil
	}
	return tx.Model(&db.GPTEmailAllocation{}).Where("email = ?", email).Updates(map[string]any{
		"status":              gptEmailStatusAssigned,
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
		Where("email = ? AND is_primary = ? AND status NOT IN ?", primaryEmail, true, []string{gptEmailStatusUserAlreadyExists, gptEmailStatusBlocked}).
		Where("EXISTS (SELECT 1 FROM gpt_email_allocations AS child WHERE child.primary_email = ? AND child.status = ?)", primaryEmail, gptEmailStatusRegistered).
		Updates(map[string]any{
			"status":     gptEmailStatusRegistered,
			"splittable": true,
		}).Error
}

func (s *gptAccountServer) findAccount(ctx context.Context, accountID string) (*db.Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	var account db.Account
	err := s.db.WithContext(ctx).First(&account, "id = ?", accountID).Error
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

func updateMap(account *pb.Account) map[string]interface{} {
	updates := map[string]interface{}{}
	if account == nil {
		return updates
	}

	if value := emailx.Normalize(account.GetEmail()); value != "" {
		updates["email"] = value
	}
	if value := account.GetPassword(); value != "" {
		updates["password"] = value
	}
	return updates
}

func (s *gptAccountServer) accountToProto(ctx context.Context, account *db.Account) (*pb.Account, error) {
	if account == nil {
		return nil, nil
	}
	out := &pb.Account{
		AccountId:           account.ID,
		Email:               account.Email,
		Password:            account.Password,
		CreatedAt:           account.CreatedAt,
		UpdatedAt:           account.UpdatedAt,
		PrimaryMailboxEmail: emailx.CanonicalPlusAlias(account.Email),
	}
	if err := s.state.apply(ctx, out); err != nil {
		return nil, err
	}
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
	case "", gptEmailStatusAvailable, gptEmailStatusOAuthPending, gptEmailStatusAuthFailed:
		return true
	case gptEmailStatusRegistered:
		return incoming == gptEmailStatusRegistered || incoming == gptEmailStatusUserAlreadyExists || incoming == gptEmailStatusBlocked
	case gptEmailStatusAssigned:
		return incoming == gptEmailStatusUserAlreadyExists || incoming == gptEmailStatusBlocked
	case gptEmailStatusUserAlreadyExists, gptEmailStatusBlocked:
		return false
	default:
		return incoming != gptEmailStatusAvailable && incoming != gptEmailStatusOAuthPending && incoming != gptEmailStatusAuthFailed
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
	if strings.TrimSpace(platformNATSURL) == "" {
		log.Fatal("PLATFORM_NATS_URL is required for account hotstream")
	}
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
	pb.RegisterGPTAccountServiceServer(grpcServer, &gptAccountServer{db: database, state: stateStore, hot: hot})
	grpchealth.RegisterServing(grpcServer)

	log.Printf("GPT account gRPC server listening on %s", listenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
