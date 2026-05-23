package activities

import (
	"sync"
)

type browserAuthFlowStore struct {
	mu    sync.Mutex
	flows map[string]*browserAuthFlow
}

func newBrowserAuthFlowStore() *browserAuthFlowStore {
	return &browserAuthFlowStore{flows: map[string]*browserAuthFlow{}}
}

func (s *browserAuthFlowStore) add(flow *browserAuthFlow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows[flow.flowID] = flow
}

func (s *browserAuthFlowStore) get(flowID string) *browserAuthFlow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flows[flowID]
}

func (s *browserAuthFlowStore) remove(flowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flows, flowID)
}
