package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"accountdb/db"
	"accountdb/pb"
)

const (
	gptEmailStatusAvailable         = "AVAILABLE"
	gptEmailStatusAssigned          = "ASSIGNED"
	gptEmailStatusRegistered        = "REGISTERED"
	gptEmailStatusOAuthPending      = "OAUTH_PENDING"
	gptEmailStatusAuthFailed        = "AUTH_FAILED"
	gptEmailStatusUserAlreadyExists = "USER_ALREADY_EXISTS"
	gptEmailStatusBlocked           = "BLOCKED"
)

type accountDatabaseServer struct {
	pb.UnimplementedAccountDatabaseServiceServer
	db *gorm.DB
}

func (s *accountDatabaseServer) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
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

	log.Printf("Created account id=%s email=%s", account.ID, redactEmail(account.Email))
	return &pb.CreateAccountResponse{Account: accountToProto(account)}, nil
}

func (s *accountDatabaseServer) GetAccount(ctx context.Context, req *pb.GetAccountRequest) (*pb.GetAccountResponse, error) {
	account, err := s.findAccount(ctx, req.GetAccountId())
	if err != nil {
		return nil, err
	}
	return &pb.GetAccountResponse{Account: accountToProto(account)}, nil
}

func (s *accountDatabaseServer) UpdateAccount(ctx context.Context, req *pb.UpdateAccountRequest) (*pb.UpdateAccountResponse, error) {
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

	account, err := s.findAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	log.Printf("Updated account id=%s status=%s", account.ID, account.Status)
	return &pb.UpdateAccountResponse{Account: accountToProto(account)}, nil
}

func (s *accountDatabaseServer) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
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
	return &pb.DeleteAccountResponse{Ack: true}, nil
}

func (s *accountDatabaseServer) ListAccounts(ctx context.Context, req *pb.ListAccountsRequest) (*pb.ListAccountsResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := s.db.WithContext(ctx).Order("created_at DESC").Limit(limit)
	if statusFilter := strings.TrimSpace(req.GetStatus()); statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	if emailFilter := normalizeEmail(req.GetEmail()); emailFilter != "" {
		query = query.Where("email = ?", emailFilter)
	}

	var accounts []db.Account
	if err := query.Find(&accounts).Error; err != nil {
		return nil, err
	}

	resp := &pb.ListAccountsResponse{Accounts: make([]*pb.Account, 0, len(accounts))}
	for i := range accounts {
		resp.Accounts = append(resp.Accounts, accountToProto(&accounts[i]))
	}
	return resp, nil
}

func (s *accountDatabaseServer) UpsertGPTEmailAllocation(ctx context.Context, req *pb.UpsertGPTEmailAllocationRequest) (*pb.UpsertGPTEmailAllocationResponse, error) {
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
	return &pb.UpsertGPTEmailAllocationResponse{Allocation: gptEmailAllocationToProto(allocation)}, nil
}

func (s *accountDatabaseServer) ListGPTEmailAllocations(ctx context.Context, req *pb.ListGPTEmailAllocationsRequest) (*pb.ListGPTEmailAllocationsResponse, error) {
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
	if primaryEmail := normalizeEmail(req.GetPrimaryEmail()); primaryEmail != "" {
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

func (s *accountDatabaseServer) ClaimGPTEmailAllocation(ctx context.Context, req *pb.ClaimGPTEmailAllocationRequest) (*pb.ClaimGPTEmailAllocationResponse, error) {
	email := normalizeEmail(req.GetEmail())
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
	return &pb.ClaimGPTEmailAllocationResponse{Claimed: true, Allocation: gptEmailAllocationToProto(allocation)}, nil
}

func (s *accountDatabaseServer) CreateGPTEmailAliasAllocation(ctx context.Context, req *pb.CreateGPTEmailAliasAllocationRequest) (*pb.CreateGPTEmailAliasAllocationResponse, error) {
	primaryEmail := normalizeEmail(req.GetPrimaryEmail())
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
				return fmt.Errorf("invalid primary email: %s", redactEmail(primary.Email))
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
		return fmt.Errorf("failed to create unique alias for %s", redactEmail(primary.Email))
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
	return &pb.CreateGPTEmailAliasAllocationResponse{Created: true, Allocation: gptEmailAllocationToProto(allocation)}, nil
}

func (s *accountDatabaseServer) MarkGPTEmailAllocationStatus(ctx context.Context, req *pb.MarkGPTEmailAllocationStatusRequest) (*pb.MarkGPTEmailAllocationStatusResponse, error) {
	email := normalizeEmail(req.GetEmail())
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
	return &pb.MarkGPTEmailAllocationStatusResponse{Allocation: gptEmailAllocationToProto(allocation)}, nil
}

func (s *accountDatabaseServer) buildAccount(input *pb.Account) (*db.Account, error) {
	if input == nil {
		input = &pb.Account{}
	}

	now := time.Now()
	account := &db.Account{
		ID:           strings.TrimSpace(input.GetAccountId()),
		Email:        strings.TrimSpace(input.GetEmail()),
		Password:     input.GetPassword(),
		Status:       strings.TrimSpace(input.GetStatus()),
		ErrorMessage: input.GetErrorMessage(),
		SessionToken: strings.TrimSpace(input.GetSessionToken()),
		AccessToken:  strings.TrimSpace(input.GetAccessToken()),
		ChargeRef:    strings.TrimSpace(input.GetChargeRef()),
		FirstName:    strings.TrimSpace(input.GetFirstName()),
		LastName:     strings.TrimSpace(input.GetLastName()),
		DOB:          strings.TrimSpace(input.GetDob()),
		Tier:         normalizeTier(input.GetTier()),
	}
	if input.PlusTrialEligible != nil {
		value := input.GetPlusTrialEligible()
		account.PlusTrialEligible = &value
	}
	if input.PlusActive != nil {
		value := input.GetPlusActive()
		account.PlusActive = &value
	}

	if account.ID == "" {
		account.ID = gofakeit.UUID()
	}
	account.Email = normalizeEmail(account.Email)
	if account.Password == "" {
		account.Password = gofakeit.Password(true, true, true, true, false, 12)
	}
	if account.FirstName == "" {
		account.FirstName = gofakeit.FirstName()
	}
	if account.LastName == "" {
		account.LastName = gofakeit.LastName()
	}
	if account.DOB == "" {
		account.DOB = randomDOB(now)
	}
	if account.Status == "" {
		account.Status = "CREATED"
	}

	return account, nil
}

func buildGPTEmailAllocation(input *pb.GPTEmailAllocation) (*db.GPTEmailAllocation, error) {
	if input == nil {
		return nil, status.Error(codes.InvalidArgument, "allocation is required")
	}
	email := normalizeEmail(input.GetEmail())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	primaryEmail := normalizeEmail(input.GetPrimaryEmail())
	if primaryEmail == "" {
		primaryEmail = canonicalEmail(email)
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
	email = normalizeEmail(email)
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
			PrimaryEmail:      canonicalEmail(email),
			IsPrimary:         canonicalEmail(email) == email,
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
	primaryEmail = normalizeEmail(primaryEmail)
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

func (s *accountDatabaseServer) findAccount(ctx context.Context, accountID string) (*db.Account, error) {
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

func (s *accountDatabaseServer) findGPTEmailAllocation(ctx context.Context, email string) (*db.GPTEmailAllocation, error) {
	email = normalizeEmail(email)
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

	if value := normalizeEmail(account.GetEmail()); value != "" {
		updates["email"] = value
	}
	if value := account.GetPassword(); value != "" {
		updates["password"] = value
	}
	if value := strings.TrimSpace(account.GetStatus()); value != "" {
		updates["status"] = value
		updates["error_message"] = account.GetErrorMessage()
	} else if account.GetErrorMessage() != "" {
		updates["error_message"] = account.GetErrorMessage()
	}
	if value := strings.TrimSpace(account.GetSessionToken()); value != "" {
		updates["session_token"] = value
	}
	if value := strings.TrimSpace(account.GetAccessToken()); value != "" {
		updates["access_token"] = value
	}
	if value := strings.TrimSpace(account.GetChargeRef()); value != "" {
		updates["charge_ref"] = value
	}
	if value := strings.TrimSpace(account.GetFirstName()); value != "" {
		updates["first_name"] = value
	}
	if value := strings.TrimSpace(account.GetLastName()); value != "" {
		updates["last_name"] = value
	}
	if value := strings.TrimSpace(account.GetDob()); value != "" {
		updates["dob"] = value
	}
	if account.PlusTrialEligible != nil {
		updates["plus_trial_eligible"] = account.GetPlusTrialEligible()
	}
	if account.PlusActive != nil {
		updates["plus_active"] = account.GetPlusActive()
	}
	if value := normalizeTier(account.GetTier()); value != "" {
		updates["tier"] = value
	}
	if account.ActivationChannel != nil {
		updates["activation_channel"] = strings.TrimSpace(account.GetActivationChannel())
	}
	return updates
}

func accountToProto(account *db.Account) *pb.Account {
	if account == nil {
		return nil
	}
	return &pb.Account{
		AccountId:         account.ID,
		Email:             account.Email,
		Password:          account.Password,
		Status:            account.Status,
		ErrorMessage:      account.ErrorMessage,
		SessionToken:      account.SessionToken,
		AccessToken:       account.AccessToken,
		ChargeRef:         account.ChargeRef,
		FirstName:         account.FirstName,
		LastName:          account.LastName,
		Dob:               account.DOB,
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
		PlusTrialEligible: account.PlusTrialEligible,
		PlusActive:        account.PlusActive,
		Tier:              account.Tier,
		ActivationChannel: &account.ActivationChannel,
	}
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

func envDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func randomDOB(now time.Time) string {
	earliest := now.AddDate(-23, 0, 1)
	latest := now.AddDate(-18, 0, 0)
	return gofakeit.DateRange(earliest, latest).Format("2006-01-02")
}

func redactEmail(email string) string {
	email = strings.TrimSpace(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "<redacted>"
	}
	local := parts[0]
	if len(local) > 2 {
		local = local[:2]
	}
	return local + "***@" + parts[1]
}

func normalizeEmail(email string) string {
	return db.NormalizeEmail(email)
}

func normalizeTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}

func canonicalEmail(email string) string {
	normalized := normalizeEmail(email)
	local, domain, ok := strings.Cut(normalized, "@")
	if !ok || local == "" || domain == "" {
		return normalized
	}
	local, _, _ = strings.Cut(local, "+")
	return local + "@" + domain
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
	log.Println("Initializing Account DB Service...")
	gofakeit.Seed(time.Now().UnixNano())
	database := db.InitDB()

	listenAddr := envDefault("LISTEN_ADDR", ":50051")
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAccountDatabaseServiceServer(grpcServer, &accountDatabaseServer{db: database})

	log.Printf("Account DB gRPC Server listening on %s", listenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
