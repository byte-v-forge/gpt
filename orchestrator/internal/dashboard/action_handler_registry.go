package dashboard

import (
	"net/http"

	"orchestrator/internal/contracts"
)

type actionHandlerMap map[string]http.HandlerFunc

type actionHandlerBinding struct {
	Profile       contracts.ActionProfile
	N8NAction     http.HandlerFunc
	WorkflowStart http.HandlerFunc
}

type actionHandlerRegistrar func(*server) []actionHandlerBinding

var (
	actionHandlerRegistrars []actionHandlerRegistrar
)

func registerActionHandlerRegistrar(registrar actionHandlerRegistrar) {
	if registrar == nil {
		return
	}
	actionHandlerRegistrars = append(actionHandlerRegistrars, registrar)
}

func (s *server) n8nActionHandlers() actionHandlerMap {
	return mergeActionHandlers(
		actionBindingsHandlerMap(s.coreActionHandlerBindings(), func(binding actionHandlerBinding) http.HandlerFunc { return binding.N8NAction }),
		s.registeredActionHandlerMap(func(binding actionHandlerBinding) http.HandlerFunc { return binding.N8NAction }),
		s.rawN8NActionHandlers(),
	)
}

func (s *server) workflowHandlers() actionHandlerMap {
	return mergeActionHandlers(
		actionBindingsHandlerMap(s.coreActionHandlerBindings(), func(binding actionHandlerBinding) http.HandlerFunc { return binding.WorkflowStart }),
		s.registeredActionHandlerMap(func(binding actionHandlerBinding) http.HandlerFunc { return binding.WorkflowStart }),
		s.rawN8NWorkflowHandlers(),
	)
}

func (s *server) registeredActionHandlerMap(handler func(actionHandlerBinding) http.HandlerFunc) actionHandlerMap {
	handlers := actionHandlerMap{}
	for _, registrar := range actionHandlerRegistrars {
		handlers = mergeActionHandlers(handlers, actionBindingsHandlerMap(registrar(s), handler))
	}
	return handlers
}

func (s *server) coreActionHandlerBindings() []actionHandlerBinding {
	return mergeActionHandlerBindings(
		n8nProbeActionBindings(s),
		n8nLoginSessionActionBindings(s),
		n8nCodexOAuthActionBindings(s),
	)
}

func actionBindingsHandlerMap(bindings []actionHandlerBinding, handler func(actionHandlerBinding) http.HandlerFunc) actionHandlerMap {
	handlers := actionHandlerMap{}
	for _, binding := range bindings {
		if binding.Profile.ActionID == "" || handler == nil {
			continue
		}
		if h := handler(binding); h != nil {
			handlers[binding.Profile.ActionID] = h
		}
	}
	return handlers
}

func n8nActionWorkflowBinding[Req any, Resp n8nStartedJobResponse](s *server, profile contracts.ActionProfile, endpoint n8nActionEndpoint, workflow n8nWorkflowStartConfig[Req, Resp]) actionHandlerBinding {
	return actionHandlerBinding{
		Profile:       profile,
		N8NAction:     n8nActionEndpointHandler(s, endpoint),
		WorkflowStart: n8nWorkflowStartHandler(s, workflow),
	}
}

func mergeActionHandlerBindings(groups ...[]actionHandlerBinding) []actionHandlerBinding {
	var out []actionHandlerBinding
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func mergeActionHandlers(groups ...actionHandlerMap) actionHandlerMap {
	out := actionHandlerMap{}
	for _, group := range groups {
		for actionID, handler := range group {
			out[actionID] = handler
		}
	}
	return out
}
