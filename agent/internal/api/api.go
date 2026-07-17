// Package api is the agent's HTTP surface: the peer allowlist middleware and
// the read handlers for M1 (/v1/info, /v1/metrics, /v1/metrics/stream).
//
// The peer middleware is a real credential check, not decoration: WireGuard's
// cryptokey routing makes the in-tunnel source IP unforgeable, so an allowlist
// on RemoteAddr is the agent's authentication (CLAUDE.md §4.2). Because of that
// there is no mTLS on top — it would be encryption over encryption.
package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"

	"github.com/x2core/mrhandly/agent/internal/audit"
	"github.com/x2core/mrhandly/agent/internal/config"
	"github.com/x2core/mrhandly/agent/internal/docker"
	"github.com/x2core/mrhandly/agent/internal/fingerprint"
	"github.com/x2core/mrhandly/agent/internal/journal"
	"github.com/x2core/mrhandly/agent/internal/protocol"
	"github.com/x2core/mrhandly/agent/internal/sampler"
	"github.com/x2core/mrhandly/agent/internal/systemd"
)

// Deps are the collaborators an API server needs.
type Deps struct {
	Config      *config.Config
	Fingerprint *fingerprint.Fingerprint
	Version     string
	Commit      string
	// Metrics is the subscription-driven source backing the SSE stream.
	Metrics *sampler.Source[protocol.Metrics]
	// OneShot returns a single fresh metrics frame for GET /v1/metrics.
	OneShot func() (protocol.Metrics, error)
	Audit   *audit.Logger

	// Services projects systemd units. Nil when systemd is unavailable on the
	// host, in which case every /v1/services route returns systemd_unavailable.
	Services *systemd.Manager
	// ServicesStream is the event-driven source backing GET /v1/services/stream.
	ServicesStream *sampler.EventSource[[]protocol.Service]
	// Journal streams journald logs for GET /v1/services/:unit/logs.
	Journal *journal.Streamer

	// Docker is the Engine client. Nil when no dialable socket is present, in
	// which case every /v1/docker route returns docker_unavailable.
	Docker *docker.Client
	// DockerWritable is false on hosts configured docker_read_only; container
	// write actions are then refused at the handler boundary.
	DockerWritable bool
}

// Server serves the agent API.
type Server struct {
	deps Deps
	mux  *http.ServeMux
}

// New builds a Server and registers its routes.
func New(deps Deps) *Server {
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /v1/info", s.handleInfo)
	s.mux.HandleFunc("GET /v1/metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /v1/metrics/stream", s.handleMetricsStream)
	s.mux.HandleFunc("GET /v1/services", s.handleServices)
	s.mux.HandleFunc("GET /v1/services/stream", s.handleServicesStream)
	s.mux.HandleFunc("GET /v1/services/{unit}", s.handleService)
	s.mux.HandleFunc("GET /v1/services/{unit}/logs", s.handleServiceLogs)
	s.mux.HandleFunc("POST /v1/services/{unit}/{action}", s.handleServiceAction)
	s.mux.HandleFunc("GET /v1/docker/containers", s.handleContainers)
	s.mux.HandleFunc("GET /v1/docker/images", s.handleImages)
	s.mux.HandleFunc("GET /v1/docker/containers/{id}", s.handleContainer)
	s.mux.HandleFunc("GET /v1/docker/containers/{id}/logs", s.handleContainerLogs)
	s.mux.HandleFunc("POST /v1/docker/containers/{id}/{action}", s.handleContainerAction)
	s.mux.HandleFunc("/", s.handleNotFound)
	return s
}

// Handler returns the fully wrapped handler: peer allowlist in front of the
// route mux.
func (s *Server) Handler() http.Handler {
	return s.peerGuard(s.mux)
}

// peerGuard rejects any request whose source IP is not in the peer allowlist.
func (s *Server) peerGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := remoteHost(r.RemoteAddr)
		addr, err := netip.ParseAddr(host)
		if err != nil || !s.deps.Config.PeerAllowed(addr) {
			s.deps.Audit.Log(host, "request", r.Method+" "+r.URL.Path, audit.ResultForbidden)
			writeError(w, http.StatusForbidden, protocol.ErrPeerForbidden, "peer not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.deps.Fingerprint.Info(s.deps.Version, s.deps.Commit))
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	m, err := s.deps.OneShot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "failed to read metrics")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, protocol.ErrNotFound, "no such endpoint")
}

// remoteHost extracts the host portion of a RemoteAddr, tolerating a missing
// port and IPv6 zone identifiers.
func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	// netip.ParseAddr accepts zones (fe80::1%wg0); leave the string intact.
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code protocol.ErrorCode, msg string) {
	writeJSON(w, status, protocol.APIError{Code: code, Message: msg})
}
