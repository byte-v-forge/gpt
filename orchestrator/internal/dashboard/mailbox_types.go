package dashboard

type upsertMailboxRequest struct {
	MailboxID    string `json:"mailbox_id"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	ProviderKey  string `json:"provider_key"`
	Status       string `json:"status"`
	AuthStatus   string `json:"auth_status"`
	LastError    string `json:"last_error"`
}

type mailboxOAuthRequest struct {
	EmailAddress string `json:"email_address"`
	OnlyMissing  bool   `json:"only_missing"`
	Limit        int32  `json:"limit"`
}

type mailboxInboxRequest struct {
	LimitPerMailbox int32  `json:"limit_per_mailbox"`
	MaxMailboxes    int32  `json:"max_mailboxes"`
	EmailAddress    string `json:"email_address"`
	ParserProfile   string `json:"parser_profile"`
}
