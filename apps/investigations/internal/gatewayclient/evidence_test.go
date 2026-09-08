package gatewayclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestEvidenceChunksReassembleEveryByte(t *testing.T) {
	content := bytes.Repeat([]byte{0, 255, 0xd0, 0x91, 13, 10, 1}, 19000)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Project-ID") != "aabbccddee" || r.Header.Get("Authorization") != "Bearer user-token" {
			t.Error("scope not forwarded")
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit != 16384 && limit != 65536 {
			t.Errorf("unexpected limit %d", limit)
		}
		end := min(offset+limit, len(content))
		w.Header().Set("X-Evidence-EOF", strconv.FormatBool(end == len(content)))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(content[offset:end])
	}))
	defer server.Close()
	client := New(Config{BaseURL: server.URL})
	for _, limit := range []int{0, 65536} {
		var result []byte
		var offset int64
		for {
			raw, err := client.ReadEvidenceContent(context.Background(), "aabbccddee", "user-token", "11111111-2222-4333-8444-555555555555", offset, limit)
			if err != nil {
				t.Fatal(err)
			}
			var chunk EvidenceChunk
			if err = json.Unmarshal(raw, &chunk); err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.StdEncoding.DecodeString(chunk.Content)
			if err != nil {
				t.Fatal(err)
			}
			if chunk.Encoding != "base64" || chunk.Size != len(decoded) || chunk.Offset != offset || chunk.NextOffset != offset+int64(len(decoded)) {
				t.Fatal(chunk)
			}
			result = append(result, decoded...)
			if chunk.EOF {
				break
			}
			offset = chunk.NextOffset
		}
		if !bytes.Equal(result, content) {
			t.Fatal("chunked evidence differs")
		}
	}
	before := calls
	for _, args := range []struct {
		offset int64
		limit  int
	}{{-1, 16}, {0, 65537}, {0, -1}, {1<<63 - 1, 16}} {
		if _, err := client.ReadEvidenceContent(context.Background(), "aabbccddee", "user-token", "11111111-2222-4333-8444-555555555555", args.offset, args.limit); err == nil {
			t.Fatal("invalid bounds accepted")
		}
	}
	if calls != before {
		t.Fatal("invalid args reached Gateway")
	}
}
func TestEvidenceClientPendingErrorAndInvalidChunk(t *testing.T) {
	mode := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == 0 {
			w.WriteHeader(409)
			io.WriteString(w, `{"error":{"code":"evidence_not_ready","message":"not ready"}}`)
			return
		}
		w.Header().Set("X-Evidence-EOF", "false")
	}))
	defer server.Close()
	client := New(Config{BaseURL: server.URL})
	_, err := client.ReadEvidenceContent(context.Background(), "aabbccddee", "token", "11111111-2222-4333-8444-555555555555", 0, 0)
	upstream, ok := err.(*HTTPError)
	if !ok || upstream.Code != "evidence_not_ready" {
		t.Fatal(err)
	}
	mode = 1
	if _, err = client.ReadEvidenceContent(context.Background(), "aabbccddee", "token", "11111111-2222-4333-8444-555555555555", 0, 0); err == nil {
		t.Fatal("zero-length nonfinal chunk accepted")
	}
}
