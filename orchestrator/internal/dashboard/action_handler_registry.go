package dashboard

import (
	"net/http"

	"orchestrator/internal/contracts"
)

type actionHandlerMap map[string]http.HandlerFunc

type actionHandlerRegistrar func(*server) actionHandlerMap

var (
	n8nActionHandlerRegistrars []actionHandlerRegistrar
	workflowHandlerRegistrars  []actionHandlerRegistrar
)

func registerN8NActionHandlerRegistrar(registrar actionHandlerRegistrar) {
	if registrar == nil {
		return
	}
	n8nActionHandlerRegistrars = append(n8nActionHandlerRegistrars, registrar)
}

func registerWorkflowHandlerRegistrar(registrar actionHandlerRegistrar) {
	if registrar == nil {
		return
	}
	workflowHandlerRegistrars = append(workflowHandlerRegistrars, registrar)
}

func (s *server) n8nActionHandlers() actionHandlerMap {
	handlers := mergeActionHandlers(s.coreN8NActionHandlers(), s.rawN8NActionHandlers())
	for _, registrar := range n8nActionHandlerRegistrars {
		handlers = mergeActionHandlers(handlers, registrar(s))
	}
	return handlers
}

func (s *server) workflowHandlers() actionHandlerMap {
	handlers := mergeActionHandlers(s.coreWorkflowHandlers(), s.rawN8NWorkflowHandlers())
	for _, registrar := range workflowHandlerRegistrars {
		handlers = mergeActionHandlers(handlers, registrar(s))
	}
	return handlers
}

func (s *server) coreN8NActionHandlers() actionHandlerMap {
	return actionHandlerMap{
		contracts.ActionProbeAccount:            s.handleProbeAccountAction,
		contracts.ActionLoginSession:            s.handleLoginAction,
		contracts.ActionLoginSessionProtocol:    s.handleLoginProtocolAction,
		contracts.ActionCodexOAuth:              s.handleCodexOAuthAction,
		contracts.ActionCodexOAuthProtocol:      s.handleCodexOAuthProtocolAction,
		contracts.ActionCodexOAuthAddPhone:      s.handleCodexOAuthAddPhoneAction,
		contracts.ActionCodexOAuthBatchAddPhone: s.handleCodexOAuthBatchAddPhoneAction,
	}
}

func (s *server) coreWorkflowHandlers() actionHandlerMap {
	return actionHandlerMap{
		contracts.ActionProbeAccount:            s.handleProbeAccount,
		contracts.ActionLoginSession:            s.handleLogin,
		contracts.ActionLoginSessionProtocol:    s.handleLoginProtocol,
		contracts.ActionCodexOAuth:              s.handleCodexOAuth,
		contracts.ActionCodexOAuthProtocol:      s.handleCodexOAuthProtocol,
		contracts.ActionCodexOAuthAddPhone:      s.handleCodexOAuthAddPhone,
		contracts.ActionCodexOAuthBatchAddPhone: s.handleCodexOAuthBatchAddPhone,
	}
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
