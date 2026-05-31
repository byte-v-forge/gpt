package actionregistry

import "orchestrator/pb"

func CatalogResponse(actions []ActionDefinition) *pb.GPTActionCatalogResponse {
	out := &pb.GPTActionCatalogResponse{Actions: make([]*pb.GPTActionDefinition, 0, len(actions))}
	for _, action := range actions {
		out.Actions = append(out.Actions, DefinitionResponse(action))
	}
	return out
}

func DefinitionResponse(action ActionDefinition) *pb.GPTActionDefinition {
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
		Engine:                  EngineResponse(action.Engine),
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

func EngineResponse(engine Engine) pb.GPTActionEngine {
	switch engine {
	case EngineN8N:
		return pb.GPTActionEngine_GPT_ACTION_ENGINE_N8N
	default:
		return pb.GPTActionEngine_GPT_ACTION_ENGINE_UNSPECIFIED
	}
}
