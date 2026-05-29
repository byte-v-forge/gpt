package gptplugin

import (
	"context"
	"fmt"
	"sync"
)

type Engine string

const (
	EngineN8N Engine = "N8N"
)

type UIButton struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Placement string `json:"placement"`
	Intent    string `json:"intent,omitempty"`
	StartPath string `json:"start_path,omitempty"`
}

type ActionAPIKind string

const (
	ActionAPIKindTyped  ActionAPIKind = ""
	ActionAPIKindRawN8N ActionAPIKind = "RAW_N8N"
)

type WorkflowDefinition struct {
	Key              string        `json:"key"`
	IDPrefix         string        `json:"-"`
	StartPath        string        `json:"start_path,omitempty"`
	N8NActionScope   string        `json:"n8n_action_scope,omitempty"`
	N8NWebhookPath   string        `json:"n8n_webhook_path,omitempty"`
	ActionPathPrefix string        `json:"action_path_prefix,omitempty"`
	ActionAPIKind    ActionAPIKind `json:"action_api_kind,omitempty"`
}

type ActionDefinition struct {
	ActionID                string             `json:"action_id"`
	DisplayName             string             `json:"display_name"`
	Owner                   string             `json:"owner"`
	Engine                  Engine             `json:"engine"`
	RequestProto            string             `json:"request_proto,omitempty"`
	ResponseProto           string             `json:"response_proto,omitempty"`
	RequiredAccountStatuses []string           `json:"required_account_statuses,omitempty"`
	BlockedAccountStatuses  []string           `json:"blocked_account_statuses,omitempty"`
	RequiredFields          []string           `json:"required_fields,omitempty"`
	Capabilities            []string           `json:"capabilities,omitempty"`
	Visibility              string             `json:"visibility,omitempty"`
	Workflow                WorkflowDefinition `json:"workflow"`
	UIButtons               []UIButton         `json:"ui_buttons,omitempty"`
	StaleSteps              []string           `json:"-"`
}

type ConfigFieldKind string

const (
	ConfigFieldString          ConfigFieldKind = "STRING"
	ConfigFieldSecret          ConfigFieldKind = "SECRET"
	ConfigFieldInteger         ConfigFieldKind = "INTEGER"
	ConfigFieldBoolean         ConfigFieldKind = "BOOLEAN"
	ConfigFieldDurationSeconds ConfigFieldKind = "DURATION_SECONDS"
	ConfigFieldURL             ConfigFieldKind = "URL"
)

type ConfigField struct {
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	Kind         ConfigFieldKind `json:"kind"`
	DefaultValue string          `json:"default_value,omitempty"`
	Required     bool            `json:"required,omitempty"`
	HelpText     string          `json:"help_text,omitempty"`
}

type ConfigSchema struct {
	PluginKey   string        `json:"plugin_key"`
	DisplayName string        `json:"display_name"`
	Owner       string        `json:"owner"`
	Fields      []ConfigField `json:"fields,omitempty"`
}

type ActionRegistry interface {
	RegisterActions(...ActionDefinition) error
	RegisterPluginConfigs(...ConfigSchema) error
}

type N8NActionCall struct {
	ActionID string
	SubPath  string
	RawJSON  []byte
}

type N8NActionInvoker interface {
	InvokeN8NAction(context.Context, N8NActionCall) (any, error)
}

type N8NWorkflowStartCall struct {
	ActionID string
	RawJSON  []byte
}

type N8NWorkflowStartResult struct {
	Response       any
	JobID          string
	AccountID      string
	TriggerPayload any
}

type N8NWorkflowTriggerFailure struct {
	ActionID       string
	JobID          string
	AccountID      string
	N8NExecutionID string
	ErrorMessage   string
	Data           map[string]any
}

type N8NWorkflowStarter interface {
	StartN8NWorkflow(context.Context, N8NWorkflowStartCall) (N8NWorkflowStartResult, error)
	FailN8NWorkflowTrigger(context.Context, N8NWorkflowTriggerFailure) error
}

type Plugin interface {
	RegisterGPTActions(ActionRegistry) error
}

type PluginFunc func(ActionRegistry) error

func (fn PluginFunc) RegisterGPTActions(registry ActionRegistry) error {
	if fn == nil {
		return nil
	}
	return fn(registry)
}

var plugins = struct {
	sync.RWMutex
	items []Plugin
}{}

func Register(plugin Plugin) {
	if plugin == nil {
		panic("gptplugin: nil plugin")
	}
	plugins.Lock()
	defer plugins.Unlock()
	plugins.items = append(plugins.items, plugin)
}

func RegisteredPlugins() []Plugin {
	plugins.RLock()
	defer plugins.RUnlock()
	out := make([]Plugin, len(plugins.items))
	copy(out, plugins.items)
	return out
}

func ApplyRegistered(registry ActionRegistry) error {
	if registry == nil {
		return fmt.Errorf("gptplugin: action registry is nil")
	}
	for _, plugin := range RegisteredPlugins() {
		if err := plugin.RegisterGPTActions(registry); err != nil {
			return err
		}
	}
	return nil
}
