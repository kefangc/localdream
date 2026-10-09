package main

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Use the same Server, store and workers as the web UI. The transport is
// stateless; generation drafts and jobs are durable application objects.
func (s *Server) mcpHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "localdream", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Local Dream creates H3 videos using local ComfyUI. Read get_capabilities and read_prompt_guide before preparing a video. Select explicit project and asset IDs; an empty project_id means the global workspace. prepare_generation saves a draft without starting GPU work. submit_generation starts GPU work only when the user has authorized generation; repeat the same draft to recover from a timeout. queued/running means unfinished. Report completion only from get_generation with status completed and an output_url. Drafts are immutable; use a new request_id for a changed plan. Tools do not call another language model.",
	})
	s.registerMCPTools(server)
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20,
		PropagateRequestCancellation: true,
	})
	return protectMCP(transport, strings.TrimSpace(os.Getenv("LOCALDREAM_MCP_TOKEN")))
}

func protectMCP(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A browser must not issue cross-origin requests to a local GPU service.
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if err != nil || u.Scheme != scheme || !strings.EqualFold(u.Host, r.Host) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				http.Error(w, "MCP origin is not allowed", http.StatusForbidden)
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "MCP cross-site request is not allowed", http.StatusForbidden)
			return
		}
		if token != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="localdream-mcp"`)
				http.Error(w, "MCP bearer token required", http.StatusUnauthorized)
				return
			}
		} else {
			peer, _, err := net.SplitHostPort(r.RemoteAddr)
			host := r.Host
			if h, _, e := net.SplitHostPort(host); e == nil {
				host = h
			}
			ip := net.ParseIP(strings.Trim(host, "[]"))
			localHost := strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
			if err != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() || !localHost || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Forwarded-Host") != "" {
				http.Error(w, "Remote MCP access requires LOCALDREAM_MCP_TOKEN", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
