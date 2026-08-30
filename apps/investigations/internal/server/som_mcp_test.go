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
