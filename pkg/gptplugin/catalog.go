package gptplugin

import "strings"

const VisibilityAccount = "account"

type ActionButtonSpec struct {
	Label     string
	Placement string
	Intent    string
	StartPath string
}

type N8NActionSpec struct {
	Owner                   string
	ActionID                string
	DisplayName             string
	WorkflowKey             string
	WorkflowIDPrefix        string
	StartPath               string
	ActionScope             string
	WebhookPath             string
	ActionPathPrefix        string
	ActionAPIKind           ActionAPIKind
	RequestProto            string
	ResponseProto           string
	Button                  ActionButtonSpec
	ExtraButtons            []ActionButtonSpec
	RequiredAccountStatuses []string
	BlockedAccountStatuses  []string
	RequiredFields          []string
	Capabilities            []string
	StaleSteps              []string
}

func BuildN8NAction(spec N8NActionSpec) ActionDefinition {
	def := ActionDefinition{
		ActionID:                spec.ActionID,
		DisplayName:             spec.DisplayName,
		Owner:                   spec.Owner,
		Engine:                  EngineN8N,
		RequestProto:            spec.RequestProto,
		ResponseProto:           spec.ResponseProto,
		Visibility:              VisibilityAccount,
		Workflow:                buildN8NWorkflow(spec),
		RequiredAccountStatuses: copyStrings(spec.RequiredAccountStatuses),
		BlockedAccountStatuses:  blockedStatusesOrDefault(spec.BlockedAccountStatuses),
		RequiredFields:          copyStrings(spec.RequiredFields),
		Capabilities:            copyStrings(spec.Capabilities),
		StaleSteps:              copyStrings(spec.StaleSteps),
	}
	if spec.Button.Label != "" || spec.Button.Placement != "" {
		def.UIButtons = append(def.UIButtons, ActionButton(spec.ActionID, spec.Button))
	}
	for _, button := range spec.ExtraButtons {
		def = WithUIButton(def, button)
	}
	return def
}

func buildN8NWorkflow(spec N8NActionSpec) WorkflowDefinition {
	return WorkflowDefinition{
		Key:              spec.WorkflowKey,
		IDPrefix:         spec.WorkflowIDPrefix,
		StartPath:        spec.StartPath,
		N8NActionScope:   spec.ActionScope,
		N8NWebhookPath:   spec.WebhookPath,
		ActionPathPrefix: spec.ActionPathPrefix,
		ActionAPIKind:    spec.ActionAPIKind,
	}
}

func ActionButton(actionID string, spec ActionButtonSpec) UIButton {
	return UIButton{
		ID:        ActionButtonID(actionID),
		Label:     spec.Label,
		Placement: spec.Placement,
		Intent:    spec.Intent,
		StartPath: spec.StartPath,
	}
}

func WithUIButton(def ActionDefinition, spec ActionButtonSpec) ActionDefinition {
	button := ActionButton(def.ActionID, spec)
	placementID := ActionButtonID(spec.Placement)
	if placementID != "" {
		button.ID = ActionButtonID(def.ActionID) + "-" + placementID
	}
	def.UIButtons = append(def.UIButtons, button)
	return def
}

func WithRequiredStatuses(def ActionDefinition, statuses ...string) ActionDefinition {
	def.RequiredAccountStatuses = append(def.RequiredAccountStatuses, statuses...)
	return def
}

func WithBlockedStatuses(def ActionDefinition, statuses ...string) ActionDefinition {
	def.BlockedAccountStatuses = append(def.BlockedAccountStatuses, statuses...)
	return def
}

func WithRequiredFields(def ActionDefinition, fields ...string) ActionDefinition {
	def.RequiredFields = append(def.RequiredFields, fields...)
	return def
}

func WithCapabilities(def ActionDefinition, capabilities ...string) ActionDefinition {
	def.Capabilities = append(def.Capabilities, capabilities...)
	return def
}

func Field(key string, label string, kind ConfigFieldKind, defaultValue string) ConfigField {
	return ConfigField{Key: key, Label: label, Kind: kind, DefaultValue: defaultValue}
}

func FieldHelp(key string, label string, kind ConfigFieldKind, defaultValue string, helpText string) ConfigField {
	field := Field(key, label, kind, defaultValue)
	field.HelpText = helpText
	return field
}

func ActionButtonID(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
}

func DefaultBlockedAccountStatuses() []string {
	return []string{AccountStatusDeactivated, AccountStatusEmailAlreadyExists, AccountStatusUserAlreadyExists}
}

func CloneActionDefinition(def ActionDefinition) ActionDefinition {
	def.RequiredAccountStatuses = copyStrings(def.RequiredAccountStatuses)
	def.BlockedAccountStatuses = copyStrings(def.BlockedAccountStatuses)
	def.RequiredFields = copyStrings(def.RequiredFields)
	def.Capabilities = copyStrings(def.Capabilities)
	def.UIButtons = append([]UIButton(nil), def.UIButtons...)
	def.StaleSteps = copyStrings(def.StaleSteps)
	return def
}

func CloneConfigSchema(schema ConfigSchema) ConfigSchema {
	schema.Fields = append([]ConfigField(nil), schema.Fields...)
	return schema
}

func blockedStatusesOrDefault(statuses []string) []string {
	out := DefaultBlockedAccountStatuses()
	out = append(out, statuses...)
	return out
}

func copyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
