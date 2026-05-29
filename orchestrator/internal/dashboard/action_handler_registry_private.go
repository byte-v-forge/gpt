//go:build private_plugins

package dashboard

import "orchestrator/internal/contracts"

func init() {
	registerN8NActionHandlerRegistrar(privateBusinessN8NActionHandlers)
	registerWorkflowHandlerRegistrar(privateBusinessWorkflowHandlers)
}

func privateBusinessN8NActionHandlers(s *server) actionHandlerMap {
	return actionHandlerMap{
		contracts.ActionRegister:         s.handleRegisterAction,
		contracts.ActionRegisterProtocol: s.handleRegisterProtocolAction,
	}
}

func privateBusinessWorkflowHandlers(s *server) actionHandlerMap {
	return actionHandlerMap{
		contracts.ActionRegister:         s.handleRegister,
		contracts.ActionRegisterProtocol: s.handleRegisterProtocol,
	}
}
