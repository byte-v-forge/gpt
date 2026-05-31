package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/protobuf/proto"
)

type n8nActionRoute func(context.Context, *http.Request) (any, error)
type n8nActionRouteFactory func() map[string]n8nActionRoute

type n8nActionEndpoint struct {
	ActionID string
	Label    string
	API      any
	Routes   n8nActionRouteFactory
}

type n8nActionHTTPError struct {
	status int
	err    error
}

func (e n8nActionHTTPError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e n8nActionHTTPError) Unwrap() error { return e.err }

func dispatchN8NEndpoint(s *server, w http.ResponseWriter, r *http.Request, endpoint n8nActionEndpoint) {
	s.dispatchN8NAction(w, r, endpoint)
}

func n8nActionEndpointHandler(s *server, endpoint n8nActionEndpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { dispatchN8NEndpoint(s, w, r, endpoint) }
}

func (s *server) dispatchN8NAction(w http.ResponseWriter, r *http.Request, cfg n8nActionEndpoint) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	label := firstNonEmptyString(cfg.Label, cfg.ActionID)
	if cfg.API == nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("n8n %s action API is not configured", label))
		return
	}
	action := s.actionSubPath(r, cfg.ActionID)
	var routes map[string]n8nActionRoute
	if cfg.Routes != nil {
		routes = cfg.Routes()
	}
	route, ok := routes[action]
	if !ok || route == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported %s action: %s", label, action))
		return
	}
	resp, err := route(r.Context(), r)
	writeN8NAction(w, resp, err)
}

func mergeN8NActionRoutes(groups ...map[string]n8nActionRoute) map[string]n8nActionRoute {
	routes := make(map[string]n8nActionRoute)
	for _, group := range groups {
		for action, route := range group {
			if route != nil {
				routes[action] = route
			}
		}
	}
	return routes
}

func n8nProtoJSONActionRoute[T proto.Message, R proto.Message](newRequest func() T, call func(context.Context, T) (R, error)) n8nActionRoute {
	return func(ctx context.Context, r *http.Request) (any, error) {
		req := newRequest()
		if err := readProtoJSON(r, req); err != nil {
			return nil, n8nActionHTTPError{status: http.StatusBadRequest, err: err}
		}
		resp, err := call(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func n8nProtoRequestActionRoute[T proto.Message](newRequest func() T, call func(context.Context, T) (any, error)) n8nActionRoute {
	return func(ctx context.Context, r *http.Request) (any, error) {
		req := newRequest()
		if err := readProtoJSON(r, req); err != nil {
			return nil, n8nActionHTTPError{status: http.StatusBadRequest, err: err}
		}
		return call(ctx, req)
	}
}

func writeN8NAction(w http.ResponseWriter, resp any, err error) {
	if err != nil {
		status := http.StatusBadGateway
		var httpErr n8nActionHTTPError
		if errors.As(err, &httpErr) && httpErr.status > 0 {
			status = httpErr.status
		}
		writeError(w, status, err)
		return
	}
	if message, ok := resp.(proto.Message); ok {
		writeProtoJSON(w, http.StatusOK, message)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
