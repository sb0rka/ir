package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestEvidenceHTTPUsesExportWriteDeadline(t *testing.T) {
	h, _ := exportHTTP(t, &exportProvider{state: "ready", content: []byte("content")})
	path := registerExport(t, h)
	r := httptest.NewRequest("GET", path+"/content", nil)
	r.Header.Set("X-Project-ID", "aabbccddee")
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "content" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.deadline.Before(time.Now().Add(59*time.Minute)) || w.deadline.After(time.Now().Add(time.Hour)) {
		t.Fatal("write deadline:", w.deadline)
	}
}

type exportSecrets struct{}

func (exportSecrets) Resolve(context.Context, string, string, ...string) (map[string]string, error) {
	return map[string]string{"cookie": "test"}, nil
}

type exportProvider struct {
	content []byte
	state   string
	broken  bool
}

func (p *exportProvider) StartEvidence(_ context.Context, _ capability.Access, ref domain.EvidenceReference) (capability.EvidenceHandle, error) {
	return capability.EvidenceHandle{Reference: ref, State: p.state, Filename: "evidence.zip", ContentType: "application/zip", TaskID: "private-task-id"}, nil
}
func (p *exportProvider) PollEvidence(_ context.Context, _ capability.Access, h capability.EvidenceHandle) (capability.EvidenceHandle, error) {
	h.State = p.state
	return h, nil
}
func (p *exportProvider) OpenEvidence(_ context.Context, _ capability.Access, _ capability.EvidenceHandle) (io.ReadCloser, error) {
	if p.broken {
		return io.NopCloser(io.MultiReader(bytes.NewReader(p.content), failingReader{})), nil
	}
	return io.NopCloser(bytes.NewReader(p.content)), nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func exportHTTP(t *testing.T, p *exportProvider) (http.Handler, map[string]map[string]bool) {
	t.Helper()
	reg, err := registry.New(registry.Provider{Source: domain.Source{Code: "nad"}, CredentialSecret: "cookie", Evidence: p})
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]map[string]bool{"aabbccddee": {"nad": true}, "1122334455": {"nad": true}}
	handler := NewHandler(config.Config{Auth: config.AuthConfig{Disabled: true}, ProjectSources: sources, Sources: map[string]config.SourceConfig{"nad": {StoreIDs: []string{"23"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)), service.New(reg, exportSecrets{}, time.Second, time.Second))
	return handler, sources
}
func exportRequest(t *testing.T, h http.Handler, method, path, project, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Project-ID", project)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func registerExport(t *testing.T, h http.Handler) string {
	t.Helper()
	w := exportRequest(t, h, "POST", "/api/v1/evidence/exports", "aabbccddee", `{"kind":"file","object_id":"file-1","ref":{"source_code":"nad","record_type":"nad_session","external_id":"flow-1","source_instance":"23","time_range":{"from":"2023-06-10T00:00:00Z","to":"2023-06-11T00:00:00Z"}}}`)
	if w.Code != 202 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "private-task-id") {
		t.Fatal("vendor ID leaked")
	}
	var result api.EvidenceExport
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return "/api/v1/evidence/exports/" + result.ExportId.String()
}
func TestEvidenceHTTPBoundedBytesAccessAndState(t *testing.T) {
	p := &exportProvider{state: "ready", content: []byte{0, 255, 1, 2, 3, 4, 5}}
	h, sources := exportHTTP(t, p)
	path := registerExport(t, h)
	for _, tc := range []struct {
		query string
		code  int
		want  []byte
		eof   string
	}{
		{"?offset=0&limit=3", 200, p.content[:3], "false"}, {"?offset=3&limit=4", 200, p.content[3:], "true"}, {"?offset=7&limit=3", 200, []byte{}, "true"}, {"?offset=8&limit=3", 416, nil, ""}, {"?offset=-1&limit=3", 400, nil, ""}, {"?limit=65537", 400, nil, ""}, {"", 200, p.content, ""},
	} {
		w := exportRequest(t, h, "GET", path+"/content"+tc.query, "aabbccddee", "")
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.query, w.Code, w.Body)
		}
		if tc.code == 200 && (!bytes.Equal(w.Body.Bytes(), tc.want) || w.Header().Get("X-Evidence-EOF") != tc.eof || w.Header().Get("Content-Type") != "application/zip" || w.Header().Get("Content-Disposition") == "") {
			t.Fatalf("invalid content: %v %v", w.Header(), w.Body.Bytes())
		}
	}
	for _, suffix := range []string{"", "/content"} {
		w := exportRequest(t, h, "GET", path+suffix, "1122334455", "")
		if w.Code != 404 {
			t.Fatal("foreign project", w.Code)
		}
	}
	sources["aabbccddee"] = map[string]bool{}
	for _, suffix := range []string{"", "/content"} {
		w := exportRequest(t, h, "GET", path+suffix, "aabbccddee", "")
		if w.Code != 404 {
			t.Fatal("revoked source", w.Code)
		}
	}
}
func TestEvidenceHTTPPendingAndPartial(t *testing.T) {
	p := &exportProvider{state: "pending", content: []byte("partial bytes")}
	h, _ := exportHTTP(t, p)
	path := registerExport(t, h)
	if w := exportRequest(t, h, "GET", path+"/content", "aabbccddee", ""); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	p.state = "partial"
	if w := exportRequest(t, h, "GET", path, "aabbccddee", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"partial"`) {
		t.Fatal(w.Code, w.Body)
	}
	if w := exportRequest(t, h, "GET", path+"/content", "aabbccddee", ""); w.Code != 200 || w.Body.String() != "partial bytes" {
		t.Fatal(w.Code, w.Body)
	}
}
func TestEvidenceHTTPInterruptedDownloadIsNotSuccessfulJSON(t *testing.T) {
	p := &exportProvider{state: "ready", content: bytes.Repeat([]byte("x"), 8192), broken: true}
	h, _ := exportHTTP(t, p)
	path := registerExport(t, h)
	server := httptest.NewServer(h)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+path+"/content", nil)
	req.Header.Set("X-Project-ID", "aabbccddee")
	resp, err := server.Client().Do(req)
	if err != nil {
		return
	} // Aborted before the first buffered bytes reached the peer.
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err == nil || (!errors.Is(err, io.ErrUnexpectedEOF) && len(raw) == 0) {
		t.Fatalf("truncation hidden: %v", err)
	}
	if bytes.Contains(raw, []byte(`"error"`)) {
		t.Fatal("JSON appended to binary response")
	}
}
