package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfigurationUsesOrdinaryTextSecretVersions(t *testing.T) {
	var version int32
	var value string
	var tagCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer generic-secrets" {
			t.Error("wrong Secrets bearer")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /projects/aabbccddee/secrets":
			items := []configurationSecret{{PublicID: "ffeeddccbbaa", SecretID: "legacy-secret", Name: "IR_CONFIG", PayloadKind: "json", Version: 1}}
			if version > 0 {
				items = append(items, configurationSecret{PublicID: "012345abcdef", SecretID: "secret-uuid", Name: "IR/SOURCE_CONFIGURATION", PayloadKind: "json", Version: version, UpdatedAt: time.Now()})
			}
			json.NewEncoder(w).Encode(map[string]any{"secrets": items})
		case "POST /projects/aabbccddee/secret":
			var request struct {
				Name  string `json:"name"`
				Kind  string `json:"payload_kind"`
				Value string `json:"secret_value"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Name != "IR/SOURCE_CONFIGURATION" || request.Kind != "json" {
				t.Error("configuration was not sent as a JSON secret")
			}
			value = request.Value
			version = 1
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(configurationSecret{PublicID: "012345abcdef", Version: version, UpdatedAt: time.Now()})
		case "POST /projects/aabbccddee/resources/012345abcdef/tag":
			var request map[string]string
			if json.NewDecoder(r.Body).Decode(&request) != nil || len(request) != 2 || request["tag_key"] != "source" || request["tag_value"] != "ir" {
				t.Error("source:ir was not attached through the ordinary Tags API")
			}
			tagCalls++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"tag_key":"source","tag_value":"ir"}`))
		case "POST /projects/aabbccddee/resources/012345abcdef/secret/versions":
			var request struct {
				Value string `json:"secret_value"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("invalid generic version request")
			}
			value = request.Value
			version++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"version_no": version, "created_at": time.Now()})
		case "POST /projects/aabbccddee/resources/012345abcdef/secret/versions/2/reveal":
			json.NewEncoder(w).Encode(map[string]any{"secret_id": "secret-uuid", "version_no": version, "value": value})
		default:
			t.Error("unexpected operation", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewConfigurationClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	empty, err := client.Read(ctx, "generic-secrets", "aabbccddee")
	if err != nil || empty.Configured || empty.Value != "[]" {
		t.Fatal("legacy IR_CONFIG was imported or empty array was not returned", err)
	}
	original := " \r\n[\r\n\t{\"name\":\"pt-nad\",\"z\" : \" exact \",\r\n\t\"a\" : \"\\u0061 <tag>\"}\r\n]\r\n"
	first, err := client.Put(ctx, "generic-secrets", "aabbccddee", []byte(original))
	if err != nil || first.Version != 1 || value != original || tagCalls != 1 {
		t.Fatal("text creation failed", err)
	}
	second, err := client.Put(ctx, "generic-secrets", "aabbccddee", []byte("\n[ ]\t\n"))
	if err != nil || second.Version != 2 || value != "\n[ ]\t\n" || tagCalls != 2 {
		t.Fatal("text version creation failed", err)
	}

	document, err := client.Read(ctx, "generic-secrets", "aabbccddee")
	if err != nil || document.Version != 2 || len(document.Values) != 0 || document.Value != value {
		t.Fatal("plain text did not become an IR document", err)
	}
	value = "opaque text from another editor: private-fixture-fragment"
	if _, err := client.Read(ctx, "generic-secrets", "aabbccddee"); err == nil || strings.Contains(err.Error(), "private-fixture-fragment") {
		t.Fatal("invalid stored JSON was accepted or leaked")
	}
}

func TestConfigurationTagFailureDoesNotCreateANewVersion(t *testing.T) {
	var versionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /projects/aabbccddee/secrets":
			json.NewEncoder(w).Encode(map[string]any{"secrets": []configurationSecret{{PublicID: "012345abcdef", SecretID: "secret-uuid", Name: "IR/SOURCE_CONFIGURATION", PayloadKind: "json", Version: 1}}})
		case "POST /projects/aabbccddee/resources/012345abcdef/tag":
			http.Error(w, "private-upstream-fragment", http.StatusForbidden)
		case "POST /projects/aabbccddee/resources/012345abcdef/secret/versions":
			versionCalls++
		default:
			t.Error("unexpected operation", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewConfigurationClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Put(context.Background(), "generic-secrets", "aabbccddee", []byte(`[]`))
	if !errors.Is(err, ErrForbidden) || versionCalls != 0 || strings.Contains(err.Error(), "private-upstream-fragment") {
		t.Fatal("tag failure was ignored or leaked", err, versionCalls)
	}
}
