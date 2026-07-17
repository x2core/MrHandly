// Package docker talks to the Docker Engine over its unix socket using plain
// net/http. docker.sock speaks ordinary HTTP/1.1, so the Docker SDK — which
// drags in half of Moby — earns nothing here (CLAUDE.md §6). The API version is
// pinned in the request path so an engine upgrade can't silently change the
// wire contract.
//
// Docker is capability-gated: transport/dial failures surface as ErrUnavailable
// so the API layer can return docker_unavailable and the UI can hide the tab.
// The socket is root-equivalent (SECURITY.md), so writes are additionally
// gated by the docker_read_only config flag at the handler boundary.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// apiVersion pins the Engine API. Bump deliberately, never implicitly.
const apiVersion = "v1.43"

// Sentinel errors the API layer switches on.
var (
	// ErrUnavailable means the socket could not be reached (absent, undialable,
	// or a transport error mid-request).
	ErrUnavailable = errors.New("docker: unavailable")
	// ErrNotFound means the engine returned 404 for a container/image.
	ErrNotFound = errors.New("docker: not found")
)

// Client is a thin Docker Engine client over a unix socket.
type Client struct {
	http     *http.Client
	rootBase string // http://unix
	base     string // http://unix/v1.43
	writable bool   // false when docker_read_only
}

// New returns a Client dialing the socket at path. writable comes from config
// (!docker_read_only) and is stamped onto every projected container.
func New(socket string, writable bool) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
		DisableCompression: true,
	}
	return &Client{
		http:     &http.Client{Transport: tr},
		rootBase: "http://unix",
		base:     "http://unix/" + apiVersion,
		writable: writable,
	}
}

// Ping reports whether the engine is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rootBase+"/_ping", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer drainClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: ping status %d", ErrUnavailable, resp.StatusCode)
	}
	return nil
}

// getJSON performs a GET and decodes the JSON body into v.
func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer drainClose(resp.Body)
	if err := httpErr(resp.StatusCode); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// post performs a POST with no body and interprets the status.
func (c *Client) post(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer drainClose(resp.Body)
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotModified:
		// 304 = already in the requested state; treat as success.
		return nil
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("docker: unexpected status %d", resp.StatusCode)
	}
}

func httpErr(status int) error {
	switch {
	case status == http.StatusNotFound:
		return ErrNotFound
	case status >= 400:
		return fmt.Errorf("docker: unexpected status %d", status)
	default:
		return nil
	}
}

func drainClose(rc io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, 1<<20))
	_ = rc.Close()
}

// ---------------------------------------------------------------------------
// Containers
// ---------------------------------------------------------------------------

type apiContainer struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

// ListContainers returns all containers (running and stopped).
func (c *Client) ListContainers(ctx context.Context) ([]protocol.Container, error) {
	var raw []apiContainer
	if err := c.getJSON(ctx, "/containers/json?all=1", &raw); err != nil {
		return nil, err
	}
	out := make([]protocol.Container, 0, len(raw))
	for _, rc := range raw {
		out = append(out, protocol.Container{
			ID:       rc.ID,
			Name:     primaryName(rc.Names),
			Image:    rc.Image,
			State:    rc.State,
			Status:   rc.Status,
			Created:  rc.Created,
			Writable: c.writable,
		})
	}
	return out, nil
}

type apiInspect struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"` // RFC3339Nano
	State   struct {
		Status string `json:"Status"`
	} `json:"State"`
	Config struct {
		Image string `json:"Image"`
		Tty   bool   `json:"Tty"`
	} `json:"Config"`
}

// InspectContainer returns one container's projected state.
func (c *Client) InspectContainer(ctx context.Context, id string) (protocol.Container, error) {
	insp, err := c.inspectRaw(ctx, id)
	if err != nil {
		return protocol.Container{}, err
	}
	return protocol.Container{
		ID:       insp.ID,
		Name:     strings.TrimPrefix(insp.Name, "/"),
		Image:    insp.Config.Image,
		State:    insp.State.Status,
		Status:   insp.State.Status,
		Created:  parseRFC3339Seconds(insp.Created),
		Writable: c.writable,
	}, nil
}

func (c *Client) inspectRaw(ctx context.Context, id string) (apiInspect, error) {
	var insp apiInspect
	if err := c.getJSON(ctx, "/containers/"+id+"/json", &insp); err != nil {
		return apiInspect{}, err
	}
	return insp, nil
}

// StartContainer / StopContainer / RestartContainer enqueue the action. The
// docker_read_only gate is enforced at the handler boundary before these run.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.post(ctx, "/containers/"+id+"/start")
}
func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.post(ctx, "/containers/"+id+"/stop")
}
func (c *Client) RestartContainer(ctx context.Context, id string) error {
	return c.post(ctx, "/containers/"+id+"/restart")
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

type apiImage struct {
	ID       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Size     int64    `json:"Size"`
	Created  int64    `json:"Created"`
}

// ListImages returns all images.
func (c *Client) ListImages(ctx context.Context) ([]protocol.Image, error) {
	var raw []apiImage
	if err := c.getJSON(ctx, "/images/json", &raw); err != nil {
		return nil, err
	}
	out := make([]protocol.Image, 0, len(raw))
	for _, ri := range raw {
		out = append(out, protocol.Image{
			ID:      ri.ID,
			Tags:    ri.RepoTags,
			Size:    ri.Size,
			Created: ri.Created,
		})
	}
	return out, nil
}

func primaryName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}
