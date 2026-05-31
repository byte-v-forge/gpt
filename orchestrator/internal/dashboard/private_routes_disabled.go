//go:build !private_plugins

package dashboard

import "net/http"

func (s *server) privateRouteBindings() []routeBinding { return nil }

func (s *server) handlePrivateJobAction(http.ResponseWriter, *http.Request, string, []string) bool {
	return false
}
