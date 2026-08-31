package server

import (
	"testing"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
)

func TestRemoteMCPURLRequiresHTTPS(t *testing.T) {
	t.Parallel()
	if _, err := remoteMCPURL(config.PromptConfig{IRBaseURL: "http://ir.example"}); err == nil {
		t.Fatal("remote HTTP MCP accepted without explicit development override")
	}
	got, err := remoteMCPURL(config.PromptConfig{IRBaseURL: "https://ir.example/base/"})
	if err != nil || got != "https://ir.example/base/mcp" {
		t.Fatalf("url=%q err=%v", got, err)
	}
	got, err = remoteMCPURL(config.PromptConfig{IRBaseURL: "http://host.docker.internal:8090", AllowInsecureMCPHTTP: true})
	if err != nil || got != "http://host.docker.internal:8090/mcp" {
		t.Fatalf("local url=%q err=%v", got, err)
	}
}

func TestInvestigationRemoteMCPServersUseProfileAccessKey(t *testing.T) {
	t.Parallel()

	servers := investigationRemoteMCPServers("https://ir.example/mcp", "4a0326c78f")
	server := servers["investigation"]
	if server.URL != "https://ir.example/mcp" || !server.Enabled {
		t.Fatalf("unexpected server: %+v", server)
	}
	if got := server.Headers["Authorization"]; got != "Bearer {env:ACCESS_KEY}" {
		t.Fatalf("authorization=%q", got)
	}
	if got := server.Headers["X-Project-ID"]; got != "4a0326c78f" {
		t.Fatalf("project=%q", got)
	}
}
