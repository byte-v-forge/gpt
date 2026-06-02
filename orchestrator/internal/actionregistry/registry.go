package actionregistry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"orchestrator/internal/contracts"
)

type Engine = gptplugin.Engine

const (
	EngineN8N = gptplugin.EngineN8N
)

type UIButton = gptplugin.UIButton

type WorkflowDefinition = gptplugin.WorkflowDefinition

type ActionDefinition = gptplugin.ActionDefinition

type ConfigFieldKind = gptplugin.ConfigFieldKind

const (
	ConfigFieldString          = gptplugin.ConfigFieldString
	ConfigFieldSecret          = gptplugin.ConfigFieldSecret
	ConfigFieldInteger         = gptplugin.ConfigFieldInteger
	ConfigFieldBoolean         = gptplugin.ConfigFieldBoolean
	ConfigFieldDurationSeconds = gptplugin.ConfigFieldDurationSeconds
	ConfigFieldURL             = gptplugin.ConfigFieldURL
)

type ConfigField = gptplugin.ConfigField

type ConfigSchema = gptplugin.ConfigSchema

type Registry struct {
	mu            sync.RWMutex
	order         []string
	actions       map[string]ActionDefinition
	configOrder   []string
	configSchemas map[string]ConfigSchema
}

func New() *Registry {
	return &Registry{
		actions:       map[string]ActionDefinition{},
		configSchemas: map[string]ConfigSchema{},
	}
}

func NewDefault(plugins ...gptplugin.Plugin) *Registry {
	registry := New()
	if err := RegisterBuiltins(registry); err != nil {
		panic(err)
	}
	if err := gptplugin.ApplyPlugins(registry, plugins...); err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Register(defs ...ActionDefinition) error {
	if r == nil {
		return fmt.Errorf("action registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, def := range defs {
		def = normalizeDefinition(def)
		if err := r.validateDefinitionLocked(def); err != nil {
			return err
		}
		r.order = append(r.order, def.ActionID)
		r.actions[def.ActionID] = def
	}
	return nil
}

func (r *Registry) RegisterActions(defs ...gptplugin.ActionDefinition) error {
	return r.Register(defs...)
}

func (r *Registry) RegisterPluginConfigs(schemas ...gptplugin.ConfigSchema) error {
	if r == nil {
		return fmt.Errorf("action registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, schema := range schemas {
		schema = normalizeConfigSchema(schema)
		if err := r.validateConfigSchemaLocked(schema); err != nil {
			return err
		}
		r.configOrder = append(r.configOrder, schema.PluginKey)
		r.configSchemas[schema.PluginKey] = schema
	}
	return nil
}

func (r *Registry) Action(actionID string) (ActionDefinition, bool) {
	if r == nil {
		return ActionDefinition{}, false
	}
	actionID = normalizeActionID(actionID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.actions[actionID]
	return gptplugin.CloneActionDefinition(def), ok
}

func (r *Registry) Actions() []ActionDefinition {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ActionDefinition, 0, len(r.order))
	for _, actionID := range r.order {
		out = append(out, gptplugin.CloneActionDefinition(r.actions[actionID]))
	}
	return out
}

func (r *Registry) PluginConfigs() []ConfigSchema {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConfigSchema, 0, len(r.configOrder))
	for _, key := range r.configOrder {
		out = append(out, gptplugin.CloneConfigSchema(r.configSchemas[key]))
	}
	return out
}

func (r *Registry) PluginConfig(pluginKey string) (ConfigSchema, bool) {
	if r == nil {
		return ConfigSchema{}, false
	}
	pluginKey = normalizePluginKey(pluginKey)
	if pluginKey == "" {
		return ConfigSchema{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	schema, ok := r.configSchemas[pluginKey]
	if !ok {
		return ConfigSchema{}, false
	}
	return gptplugin.CloneConfigSchema(schema), true
}

func (r *Registry) PluginDefaults(pluginKey string) map[string]string {
	schema, ok := r.PluginConfig(pluginKey)
	if !ok || len(schema.Fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(schema.Fields))
	for _, field := range schema.Fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(field.DefaultValue)
	}
	return out
}

func (r *Registry) JobClaimStaleSteps() []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, def := range r.Actions() {
		for _, step := range def.StaleSteps {
			step = strings.TrimSpace(step)
			if step == "" || seen[step] {
				continue
			}
			seen[step] = true
			out = append(out, step)
		}
	}
	return out
}

func (r *Registry) HasCapability(capability string) bool {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return false
	}
	for _, def := range r.Actions() {
		for _, item := range def.Capabilities {
			if item == capability {
				return true
			}
		}
	}
	return false
}

func (r *Registry) WorkflowID(actionID string, jobID string) (string, bool) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return "", false
	}
	def, ok := r.Action(actionID)
	if !ok || strings.TrimSpace(def.Workflow.IDPrefix) == "" {
		return "", false
	}
	return def.Workflow.IDPrefix + jobID, true
}

func (r *Registry) WorkflowStartRoutes() map[string]ActionDefinition {
	routes := map[string]ActionDefinition{}
	for _, def := range r.Actions() {
		path := strings.TrimSpace(def.Workflow.StartPath)
		if path == "" {
			continue
		}
		routes[path] = def
	}
	return routes
}

func (r *Registry) validateDefinitionLocked(def ActionDefinition) error {
	if def.ActionID == "" {
		return fmt.Errorf("action_id is required")
	}
	if _, exists := r.actions[def.ActionID]; exists {
		return fmt.Errorf("action already registered: %s", def.ActionID)
	}
	if def.DisplayName == "" {
		return fmt.Errorf("display_name is required for action %s", def.ActionID)
	}
	if def.RequestProto == "" {
		return fmt.Errorf("request_proto is required for action %s", def.ActionID)
	}
	if def.ResponseProto == "" {
		return fmt.Errorf("response_proto is required for action %s", def.ActionID)
	}
	if def.Visibility == "" {
		return fmt.Errorf("visibility is required for action %s", def.ActionID)
	}
	if len(def.Capabilities) == 0 {
		return fmt.Errorf("capabilities are required for action %s", def.ActionID)
	}
	if len(def.UIButtons) == 0 {
		return fmt.Errorf("ui_buttons are required for action %s", def.ActionID)
	}
	if def.Workflow.Key == "" {
		return fmt.Errorf("workflow key is required for action %s", def.ActionID)
	}
	if def.Workflow.StartPath == "" {
		return fmt.Errorf("workflow start_path is required for action %s", def.ActionID)
	}
	if def.Engine != EngineN8N {
		return fmt.Errorf("unsupported engine %q for action %s", def.Engine, def.ActionID)
	}
	if def.Engine == EngineN8N {
		if def.Workflow.N8NActionScope == "" {
			return fmt.Errorf("n8n action scope is required for action %s", def.ActionID)
		}
		if def.Workflow.N8NWebhookPath == "" {
			return fmt.Errorf("n8n webhook path is required for action %s", def.ActionID)
		}
	}
	for _, button := range def.UIButtons {
		if button.ID == "" {
			return fmt.Errorf("ui button id is required for action %s", def.ActionID)
		}
		if button.Label == "" {
			return fmt.Errorf("ui button label is required for action %s", def.ActionID)
		}
		if button.Placement == "" {
			return fmt.Errorf("ui button placement is required for action %s", def.ActionID)
		}
	}
	for _, existing := range r.actions {
		if existing.Workflow.Key == def.Workflow.Key {
			return fmt.Errorf("workflow key %q already registered by action %s", def.Workflow.Key, existing.ActionID)
		}
		if existing.Workflow.StartPath != "" && existing.Workflow.StartPath == def.Workflow.StartPath {
			return fmt.Errorf("workflow start_path %q already registered by action %s", def.Workflow.StartPath, existing.ActionID)
		}
		if existing.Workflow.ActionPathPrefix != "" && existing.Workflow.ActionPathPrefix == def.Workflow.ActionPathPrefix {
			return fmt.Errorf("workflow action_path_prefix %q already registered by action %s", def.Workflow.ActionPathPrefix, existing.ActionID)
		}
	}
	return nil
}

func (r *Registry) validateConfigSchemaLocked(schema ConfigSchema) error {
	if schema.PluginKey == "" {
		return fmt.Errorf("plugin_key is required")
	}
	if schema.DisplayName == "" {
		return fmt.Errorf("display_name is required for plugin config %s", schema.PluginKey)
	}
	if _, exists := r.configSchemas[schema.PluginKey]; exists {
		return fmt.Errorf("plugin config already registered: %s", schema.PluginKey)
	}
	seen := map[string]bool{}
	for _, field := range schema.Fields {
		if field.Key == "" {
			return fmt.Errorf("plugin config field key is required for plugin %s", schema.PluginKey)
		}
		if seen[field.Key] {
			return fmt.Errorf("plugin config field %q is duplicated for plugin %s", field.Key, schema.PluginKey)
		}
		seen[field.Key] = true
		if field.Label == "" {
			return fmt.Errorf("plugin config field label is required for %s.%s", schema.PluginKey, field.Key)
		}
		if field.Kind == "" {
			return fmt.Errorf("plugin config field kind is required for %s.%s", schema.PluginKey, field.Key)
		}
	}
	return nil
}

func normalizeDefinition(def ActionDefinition) ActionDefinition {
	def.ActionID = normalizeActionID(def.ActionID)
	def.Owner = strings.TrimSpace(def.Owner)
	if def.Owner == "" {
		def.Owner = "gpt"
	}
	def.Engine = Engine(strings.ToUpper(strings.TrimSpace(string(def.Engine))))
	if def.Engine == "" {
		def.Engine = EngineN8N
	}
	def.Workflow.Key = strings.TrimSpace(def.Workflow.Key)
	def.Workflow.IDPrefix = strings.TrimSpace(def.Workflow.IDPrefix)
	def.Workflow.StartPath = normalizePath(def.Workflow.StartPath)
	def.Workflow.N8NActionScope = strings.TrimSpace(def.Workflow.N8NActionScope)
	def.Workflow.N8NWebhookPath = strings.Trim(strings.TrimSpace(def.Workflow.N8NWebhookPath), "/")
	def.Workflow.ActionPathPrefix = normalizePath(def.Workflow.ActionPathPrefix)
	def.Workflow.ActionAPIKind = normalizeActionAPIKind(def.Workflow.ActionAPIKind)
	def.DisplayName = strings.TrimSpace(def.DisplayName)
	def.RequestProto = strings.TrimSpace(def.RequestProto)
	def.ResponseProto = strings.TrimSpace(def.ResponseProto)
	def.Visibility = strings.TrimSpace(def.Visibility)
	def.RequiredAccountStatuses = compactStrings(def.RequiredAccountStatuses)
	def.BlockedAccountStatuses = compactStrings(def.BlockedAccountStatuses)
	def.RequiredFields = compactStrings(def.RequiredFields)
	def.Capabilities = compactStrings(def.Capabilities)
	def.StaleSteps = compactStrings(def.StaleSteps)
	for i := range def.UIButtons {
		def.UIButtons[i].ID = strings.TrimSpace(def.UIButtons[i].ID)
		def.UIButtons[i].Label = strings.TrimSpace(def.UIButtons[i].Label)
		def.UIButtons[i].Placement = strings.TrimSpace(def.UIButtons[i].Placement)
		def.UIButtons[i].Intent = strings.TrimSpace(def.UIButtons[i].Intent)
		def.UIButtons[i].StartPath = normalizePath(def.UIButtons[i].StartPath)
		if def.UIButtons[i].StartPath == "" {
			def.UIButtons[i].StartPath = def.Workflow.StartPath
		}
	}
	return def
}

func normalizeConfigSchema(schema ConfigSchema) ConfigSchema {
	schema.PluginKey = strings.ToLower(strings.TrimSpace(schema.PluginKey))
	schema.DisplayName = strings.TrimSpace(schema.DisplayName)
	schema.Owner = strings.TrimSpace(schema.Owner)
	if schema.Owner == "" {
		schema.Owner = "gpt"
	}
	for i := range schema.Fields {
		schema.Fields[i].Key = strings.TrimSpace(schema.Fields[i].Key)
		schema.Fields[i].Label = strings.TrimSpace(schema.Fields[i].Label)
		schema.Fields[i].Kind = ConfigFieldKind(strings.ToUpper(strings.TrimSpace(string(schema.Fields[i].Kind))))
		if schema.Fields[i].Kind == "" {
			schema.Fields[i].Kind = ConfigFieldString
		}
		schema.Fields[i].DefaultValue = strings.TrimSpace(schema.Fields[i].DefaultValue)
		schema.Fields[i].HelpText = strings.TrimSpace(schema.Fields[i].HelpText)
	}
	return schema
}

func normalizePluginKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeActionAPIKind(value gptplugin.ActionAPIKind) gptplugin.ActionAPIKind {
	switch strings.ToUpper(strings.TrimSpace(string(value))) {
	case string(gptplugin.ActionAPIKindRawN8N):
		return gptplugin.ActionAPIKindRawN8N
	default:
		return gptplugin.ActionAPIKindTyped
	}
}

func normalizeActionID(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

type builtinN8NActionSpec struct {
	ActionID         string
	DisplayName      string
	WorkflowIDPrefix string
	StartPath        string
	RequestProto     string
	ResponseProto    string
	ButtonLabel      string
	Placement        string
	RequiredStatuses []string
	RequiredFields   []string
	Capabilities     []string
}

func BuiltinActions() []ActionDefinition {
	return []ActionDefinition{
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:         contracts.ActionProbeAccount,
			DisplayName:      "Probe Account",
			WorkflowIDPrefix: "probe-",
			StartPath:        "/workflows/probe",
			RequestProto:     "orchestrator.ProbeAccountRequest",
			ResponseProto:    "orchestrator.ProbeAccountResponse",
			ButtonLabel:      "探测账号",
			Placement:        "account_header_tools",
			RequiredStatuses: []string{gptplugin.AccountStatusRegistered, gptplugin.AccountStatusActivated},
			Capabilities:     []string{contracts.CapabilityAccountProbe, contracts.CapabilityN8NWorkflow},
		}),
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:       contracts.ActionLoginSession,
			DisplayName:    "Login Session",
			RequestProto:   "orchestrator.LoginAccountRequest",
			ResponseProto:  "orchestrator.LoginAccountResponse",
			ButtonLabel:    "浏览器登录",
			Placement:      "account_header_browser",
			RequiredFields: []string{"email", "password"},
			Capabilities:   []string{contracts.CapabilityLogin, contracts.CapabilityBrowserAuth, contracts.CapabilityN8NWorkflow},
		}),
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:       contracts.ActionLoginSessionProtocol,
			DisplayName:    "Login Session Protocol",
			RequestProto:   "orchestrator.LoginAccountRequest",
			ResponseProto:  "orchestrator.LoginAccountResponse",
			ButtonLabel:    "协议登录",
			Placement:      "account_header_protocol",
			RequiredFields: []string{"email", "password"},
			Capabilities:   []string{contracts.CapabilityLogin, contracts.CapabilityProtocolAuth, contracts.CapabilityN8NWorkflow},
		}),
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:       contracts.ActionCodexOAuth,
			DisplayName:    "Codex OAuth",
			RequestProto:   "orchestrator.CodexOAuthRequest",
			ResponseProto:  "orchestrator.CodexOAuthResponse",
			ButtonLabel:    "浏览器 auth.json",
			Placement:      "account_header_browser",
			RequiredFields: []string{"email", "password"},
			Capabilities:   []string{contracts.CapabilityCodexOAuth, contracts.CapabilityBrowserAuth, contracts.CapabilityN8NWorkflow},
		}),
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:       contracts.ActionCodexOAuthProtocol,
			DisplayName:    "Codex OAuth Protocol",
			RequestProto:   "orchestrator.CodexOAuthRequest",
			ResponseProto:  "orchestrator.CodexOAuthResponse",
			ButtonLabel:    "协议 auth.json",
			Placement:      "account_header_protocol",
			RequiredFields: []string{"email", "password"},
			Capabilities:   []string{contracts.CapabilityCodexOAuth, contracts.CapabilityProtocolAuth, contracts.CapabilityN8NWorkflow},
		}),
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:       contracts.ActionCodexOAuthAddPhone,
			DisplayName:    "Codex OAuth Add Phone",
			RequestProto:   "orchestrator.CodexOAuthAddPhoneRequest",
			ResponseProto:  "orchestrator.CodexOAuthAddPhoneResponse",
			ButtonLabel:    "Add Phone",
			Placement:      "account_header_tools",
			RequiredFields: []string{"email", "password"},
			Capabilities:   []string{contracts.CapabilityCodexOAuth, contracts.CapabilityPhoneBinding, contracts.CapabilityN8NWorkflow},
		}),
		builtinN8NAction(builtinN8NActionSpec{
			ActionID:      contracts.ActionCodexOAuthBatchAddPhone,
			DisplayName:   "Codex OAuth Batch Add Phone",
			StartPath:     "/workflows/codex-oauth-add-phone/batch",
			RequestProto:  "orchestrator.CodexOAuthBatchAddPhoneRequest",
			ResponseProto: "orchestrator.CodexOAuthBatchAddPhoneResponse",
			ButtonLabel:   "批量 Add Phone",
			Placement:     "account_bulk",
			Capabilities:  []string{contracts.CapabilityCodexOAuth, contracts.CapabilityPhoneBinding, contracts.CapabilityN8NWorkflow},
		}),
	}
}

func builtinN8NAction(spec builtinN8NActionSpec) ActionDefinition {
	profile := contracts.ResolveActionProfile(spec.ActionID)
	workflowLabel := profile.WorkflowLabelOrDefault()
	apiLabel := profile.APILabelOrDefault()
	workflowIDPrefix := strings.TrimSpace(spec.WorkflowIDPrefix)
	if workflowIDPrefix == "" {
		workflowIDPrefix = workflowLabel + "-"
	}
	startPath := strings.TrimSpace(spec.StartPath)
	if startPath == "" {
		startPath = "/workflows/" + apiLabel
	}
	return gptplugin.BuildN8NAction(gptplugin.N8NActionSpec{
		Owner:                   "gpt",
		ActionID:                profile.ActionID,
		DisplayName:             spec.DisplayName,
		WorkflowKey:             workflowLabel,
		WorkflowIDPrefix:        workflowIDPrefix,
		StartPath:               startPath,
		ActionScope:             workflowLabel,
		WebhookPath:             "gpt/" + workflowLabel,
		ActionPathPrefix:        "/actions/" + workflowLabel,
		RequestProto:            spec.RequestProto,
		ResponseProto:           spec.ResponseProto,
		Button:                  gptplugin.ActionButtonSpec{Label: spec.ButtonLabel, Placement: spec.Placement},
		RequiredAccountStatuses: spec.RequiredStatuses,
		RequiredFields:          spec.RequiredFields,
		Capabilities:            spec.Capabilities,
		StaleSteps:              defaultJobClaimStaleSteps(),
	})
}

func BuiltinPluginConfigs() []ConfigSchema {
	return []ConfigSchema{
		{
			PluginKey:   "browser_auth",
			DisplayName: "Browser Auth",
			Owner:       "gpt",
			Fields: []ConfigField{
				gptplugin.Field("browser_auth_proxy_ref", "Browser Auth Proxy Ref", ConfigFieldString, "register"),
				gptplugin.Field("browser_auth_locale", "Browser Auth Locale", ConfigFieldString, "en-US"),
				gptplugin.Field("browser_auth_accept_language", "Browser Auth Accept Language", ConfigFieldString, "en-US,en;q=0.9"),
				gptplugin.Field("browser_auth_timezone", "Browser Auth Timezone", ConfigFieldString, ""),
				gptplugin.Field("browser_auth_window_width", "Browser Auth Window Width", ConfigFieldInteger, "1365"),
				gptplugin.Field("browser_auth_window_height", "Browser Auth Window Height", ConfigFieldInteger, "768"),
				gptplugin.Field("browser_auth_session_ttl_seconds", "Browser Auth Session TTL Seconds", ConfigFieldDurationSeconds, "1800"),
				gptplugin.Field("browser_auth_command_timeout_seconds", "Browser Auth Command Timeout Seconds", ConfigFieldDurationSeconds, "120"),
				gptplugin.Field("browser_auth_block_images", "Browser Auth Block Images", ConfigFieldBoolean, "false"),
				gptplugin.Field("browser_auth_geoip", "Browser Auth GeoIP", ConfigFieldBoolean, "true"),
				gptplugin.Field("browser_auth_humanize", "Browser Auth Humanize", ConfigFieldString, "true"),
			},
		},
		{
			PluginKey:   "codex_oauth",
			DisplayName: "Codex OAuth",
			Owner:       "gpt",
			Fields: []ConfigField{
				gptplugin.Field("client_id", "Client ID", ConfigFieldString, "app_EMoamEEZ73f0CkXaXp7hrann"),
				gptplugin.Field("redirect_uri", "Redirect URI", ConfigFieldURL, "http://localhost:1455/auth/callback"),
				gptplugin.Field("auth_url", "Auth URL", ConfigFieldURL, "https://auth.openai.com/oauth/authorize"),
				gptplugin.Field("token_url", "Token URL", ConfigFieldURL, "https://auth.openai.com/oauth/token"),
				gptplugin.Field("token_proxy_url", "Token Proxy URL", ConfigFieldString, ""),
				gptplugin.Field("protocol_proxy_url", "Protocol Proxy URL", ConfigFieldString, ""),
				gptplugin.Field("protocol_proxy_runtime_http_addr", "Protocol Proxy Runtime HTTP", ConfigFieldString, ""),
				gptplugin.Field("protocol_tls_profile", "Protocol TLS Profile", ConfigFieldString, "chrome_146"),
				gptplugin.Field("protocol_session_dump_enabled", "Protocol Session Dump", ConfigFieldBoolean, "true"),
				gptplugin.Field("scope", "Scope", ConfigFieldString, "openid profile email offline_access api.connectors.read api.connectors.invoke"),
				gptplugin.Field("phone_label", "Phone Label", ConfigFieldString, contracts.WorkflowLabelForAction(contracts.ActionCodexOAuthAddPhone)),
				gptplugin.Field("phone_profile_key", "Phone Profile Key", ConfigFieldString, "openai-th"),
				gptplugin.Field("phone_max_reuse_count", "Phone Max Reuse Count", ConfigFieldInteger, "3"),
				gptplugin.Field("phone_country_iso2", "Phone Country ISO2", ConfigFieldString, "TH"),
				gptplugin.Field("phone_country_calling_code", "Phone Country Calling Code", ConfigFieldString, "66"),
				gptplugin.Field("phone_wait_seconds", "Phone Wait Seconds", ConfigFieldDurationSeconds, "120"),
				gptplugin.Field("phone_min_reuse_remaining_seconds", "Phone Min Reuse Remaining Seconds", ConfigFieldDurationSeconds, "180"),
			},
		},
	}
}

func RegisterBuiltins(registry gptplugin.ActionRegistry) error {
	if err := registry.RegisterActions(BuiltinActions()...); err != nil {
		return err
	}
	return registry.RegisterPluginConfigs(BuiltinPluginConfigs()...)
}

func defaultJobClaimStaleSteps() []string {
	return []string{
		"claimed",
		contracts.StepProbePlusTrial,
		contracts.StepProbeTier,
		contracts.StepCodexOAuthAcquirePhone,
		contracts.StepCodexOAuthProtocolStart,
		contracts.StepCodexOAuthProtocolDetect,
		contracts.StepCodexOAuthProtocolEmail,
		contracts.StepCodexOAuthProtocolPassword,
		contracts.StepCodexOAuthProtocolEmailOTP,
		contracts.StepCodexOAuthProtocolAddPhone,
		contracts.StepCodexOAuthProtocolComplete,
		contracts.StepCodexOAuthBrowserStart,
		contracts.StepCodexOAuthBrowserDetect,
		contracts.StepCodexOAuthBrowserEmail,
		contracts.StepCodexOAuthBrowserPassword,
		contracts.StepCodexOAuthBrowserEmailOTP,
		contracts.StepCodexOAuthBrowserAddPhone,
		contracts.StepCodexOAuthBrowserComplete,
		contracts.StepCodexOAuthReleasePhone,
	}
}
