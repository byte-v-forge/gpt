package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/byte-v-forge/common-lib/grpcclient"
	"github.com/byte-v-forge/common-lib/protojsonhttp"
	"github.com/byte-v-forge/common-lib/randx"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) handleProxyRuntime(w http.ResponseWriter, r *http.Request) {
	s.proxyRuntimeProxy.ServeHTTP(w, r)
}

func newHTTPReverseProxy(target string) http.Handler {
	parsed, err := url.Parse(target)
	if err != nil {
		log.Fatalf("parse reverse proxy target %q: %v", target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(parsed)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		writeError(w, http.StatusBadGateway, err)
	}
	return proxy
}

func (s *server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func readProtoJSON(r *http.Request, dst proto.Message) error {
	return protojsonhttp.ReadRequest(r, dst)
}

func serveProtoAction(w http.ResponseWriter, r *http.Request, req proto.Message, call func() (proto.Message, error)) {
	if err := readProtoJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := call()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProtoJSON(w http.ResponseWriter, status int, value proto.Message) {
	_ = protojsonhttp.WriteResponse(w, status, value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

type startedResponse interface {
	GetStarted() bool
	GetErrorMessage() string
}

func writeStartedJSON(w http.ResponseWriter, resp startedResponse) {
	statusCode := http.StatusAccepted
	if !resp.GetStarted() || resp.GetErrorMessage() != "" {
		statusCode = http.StatusBadGateway
	}
	writeJSON(w, statusCode, resp)
}

func newGRPCClient(addr string) (*grpc.ClientConn, error) {
	return grpcclient.NewInsecurePassthrough(addr)
}

func randomID() string {
	if value, err := randx.Hex(16); err == nil {
		return value
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
