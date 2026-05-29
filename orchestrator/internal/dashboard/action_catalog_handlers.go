package dashboard

import (
	"net/http"

	"orchestrator/internal/actionregistry"
	"orchestrator/pb"
)

func (s *server) handleActionCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeProtoJSON(w, http.StatusOK, actionCatalogProto(s.actionRegistry.Actions()))
}

func actionCatalogProto(actions []actionregistry.ActionDefinition) *pb.GPTActionCatalogResponse {
	out := &pb.GPTActionCatalogResponse{Actions: make([]*pb.GPTActionDefinition, 0, len(actions))}
	for _, action := range actions {
		out.Actions = append(out.Actions, actionDefinitionProto(action))
	}
	return out
}

func actionDefinitionProto(action actionregistry.ActionDefinition) *pb.GPTActionDefinition {
	buttons := make([]*pb.GPTActionButton, 0, len(action.UIButtons))
	for _, button := range action.UIButtons {
		buttons = append(buttons, &pb.GPTActionButton{
			Id:        button.ID,
			Label:     button.Label,
			Placement: button.Placement,
			Intent:    button.Intent,
			StartPath: button.StartPath,
		})
	}
	return &pb.GPTActionDefinition{
		ActionId:                action.ActionID,
		DisplayName:             action.DisplayName,
		Owner:                   action.Owner,
		Engine:                  actionEngineProto(action.Engine),
		RequestProto:            action.RequestProto,
		ResponseProto:           action.ResponseProto,
		RequiredAccountStatuses: append([]string(nil), action.RequiredAccountStatuses...),
		BlockedAccountStatuses:  append([]string(nil), action.BlockedAccountStatuses...),
		RequiredFields:          append([]string(nil), action.RequiredFields...),
		Capabilities:            append([]string(nil), action.Capabilities...),
		Visibility:              action.Visibility,
		Workflow: &pb.GPTWorkflowDefinition{
			Key:              action.Workflow.Key,
			StartPath:        action.Workflow.StartPath,
			N8NActionScope:   action.Workflow.N8NActionScope,
			N8NWebhookPath:   action.Workflow.N8NWebhookPath,
			ActionPathPrefix: action.Workflow.ActionPathPrefix,
		},
		UiButtons: buttons,
	}
}

func actionEngineProto(engine actionregistry.Engine) pb.GPTActionEngine {
	switch engine {
	case actionregistry.EngineN8N:
		return pb.GPTActionEngine_GPT_ACTION_ENGINE_N8N
	default:
		return pb.GPTActionEngine_GPT_ACTION_ENGINE_UNSPECIFIED
	}
}
