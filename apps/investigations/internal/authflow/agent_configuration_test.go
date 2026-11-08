package authflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/packages/common/agentaccess"
)

type configurationSyncStore struct {
	*agentAccessStore
	mu                  sync.Mutex
	held                bool
	puts                int
	putError, lockError error
}

func (d *configurationSyncStore) Lock(_ context.Context, project string) (func(), error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if project != agentAccessProject || d.held {
		return nil, errors.New("unexpected configuration lock")
	}
	if d.lockError != nil {
		return nil, d.lockError
	}
	d.held = true
	d.locks++
	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.held = false
		d.releases++
	}, nil
}

func (d *configurationSyncStore) PutConfiguration(_ context.Context, value agentaccess.Configuration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.held || value.ProjectID != agentAccessProject {
		return errors.New("configuration persisted outside project lock")
	}
	d.puts++
	if d.putError != nil {
		return d.putError
	}
	d.configuration = value
	return nil
}

type configurationSyncRemote struct {
	mu                      sync.Mutex
	version                 int32
	value                   string
	missing, requireLock    bool
	listStatus, writeStatus int
	reads, reveals, writes  int
}

func configurationSyncFixture(t *testing.T) (*Service, *configurationSyncStore, *configurationSyncRemote, string) {
	t.Helper()
	s, base, _, user := agentAccessFixture(t)
	db := &configurationSyncStore{agentAccessStore: base}
	s.agentDB = db
	remote := &configurationSyncRemote{version: 2, value: `[{"name":"pt-nad","sessionid":"original-private-cookie"}]`, requireLock: true}
	secretID := uuid.NewString()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/projects/"+agentAccessProject {
			_ = json.NewEncoder(w).Encode(map[string]any{"public_id": agentAccessProject, "is_active": true})
			return
		}
		remote.mu.Lock()
		defer remote.mu.Unlock()
		db.mu.Lock()
		held := db.held
		db.mu.Unlock()
		if remote.requireLock && !held {
			t.Error("Platform configuration accessed before acquiring the project lock")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /projects/" + agentAccessProject + "/secrets":
			remote.reads++
			if remote.listStatus != 0 {
				w.WriteHeader(remote.listStatus)
				return
			}
			items := []map[string]any{}
			if !remote.missing {
				items = append(items, map[string]any{"public_id": "012345abcdef", "secret_id": secretID, "name": "IR/SOURCE_CONFIGURATION", "payload_kind": "json", "current_version_no": remote.version, "updated_at": time.Now().UTC()})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": items})
		case "POST /projects/" + agentAccessProject + "/resources/012345abcdef/secret/versions/2/reveal", "POST /projects/" + agentAccessProject + "/resources/012345abcdef/secret/versions/3/reveal":
			remote.reveals++
			_ = json.NewEncoder(w).Encode(map[string]any{"secret_id": secretID, "version_no": remote.version, "value": remote.value})
		case "POST /projects/" + agentAccessProject + "/resources/012345abcdef/tag":
			_, _ = w.Write([]byte(`{}`))
		case "POST /projects/" + agentAccessProject + "/resources/012345abcdef/secret/versions":
			if remote.writeStatus != 0 {
				w.WriteHeader(remote.writeStatus)
				return
			}
			var body struct {
				Value string `json:"secret_value"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid source configuration write")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			remote.writes++
			remote.version++
			remote.value = body.Value
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"version_no": remote.version, "created_at": time.Now().UTC()})
		default:
			t.Error("unexpected configuration request", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fixture.Close)
	s.cfg.Platform.APIBaseURL = fixture.URL
	return s, db, remote, user
}

func TestConfigurationReloadSynchronizesEncryptedProjectSnapshot(t *testing.T) {
	s, db, remote, user := configurationSyncFixture(t)
	ctx := context.Background()
	first, err := s.Configuration(ctx, user, agentAccessProject)
	if err != nil || first.Version != 2 {
		t.Fatal("configuration read failed", err)
	}
	loaded, err := s.agentCipher.Open(db.configuration)
	if err != nil || loaded.Value != first.Value || loaded.Values["pt-nad"]["sessionid"] != "original-private-cookie" || bytes.Contains(db.configuration.EncryptedConfig, []byte("original-private-cookie")) {
		t.Fatal("configuration was not stored encrypted and bound to its project", err)
	}
	remote.mu.Lock()
	remote.version = 3
	remote.value = `[{"name":"pt-nad","sessionid":"new-private-cookie"}]`
	remote.mu.Unlock()
	if _, err := s.Configuration(ctx, user, agentAccessProject); err != nil {
		t.Fatal("refresh failed", err)
	}
	loaded, err = s.agentCipher.Open(db.configuration)
	if err != nil || loaded.Version != 3 || loaded.Values["pt-nad"]["sessionid"] != "new-private-cookie" {
		t.Fatal("refresh kept stale credentials", err)
	}
	remote.mu.Lock()
	remote.missing = true
	remote.mu.Unlock()
	empty, err := s.Configuration(ctx, user, agentAccessProject)
	if err != nil || empty.Configured || empty.Value != "[]" {
		t.Fatal("missing Platform secret did not produce an empty configuration", err)
	}
	loaded, err = s.agentCipher.Open(db.configuration)
	if err != nil || loaded.Configured || loaded.Value != "[]" || len(loaded.Values) != 0 {
		t.Fatal("reload did not clear deleted credentials", err)
	}
	if db.puts != 3 || db.locks != 3 || db.releases != 3 {
		t.Fatal("reload did not synchronize under the project lock", db.puts, db.locks, db.releases)
	}
}

func TestConfigurationSaveSynchronizesWithoutRevealingPlatformSecret(t *testing.T) {
	s, db, remote, user := configurationSyncFixture(t)
	value := "[\n  {\"name\":\"pt-nad\",\"sessionid\":\"saved-private-cookie\"}\n]"
	metadata, err := s.PutConfiguration(context.Background(), user, agentAccessProject, []byte(value))
	if err != nil || metadata.Version != 3 {
		t.Fatal("configuration save failed", err)
	}
	loaded, err := s.agentCipher.Open(db.configuration)
	if err != nil || loaded.Value != value || loaded.Version != metadata.Version || loaded.SecretID != metadata.SecretID || bytes.Contains(db.configuration.EncryptedConfig, []byte("saved-private-cookie")) {
		t.Fatal("save did not synchronize the exact encrypted document", err)
	}
	if db.puts != 1 || db.locks != 1 || db.releases != 1 || remote.writes != 1 || remote.reveals != 0 {
		t.Fatal("save skipped synchronization, leaked its lock or performed an unnecessary reveal")
	}
}

func TestConfigurationSyncFailuresPreserveStoredCredentials(t *testing.T) {
	for _, scenario := range []string{"read denied", "write denied", "local persistence failed", "lock failed"} {
		t.Run(scenario, func(t *testing.T) {
			s, db, remote, user := configurationSyncFixture(t)
			if _, err := s.Configuration(context.Background(), user, agentAccessProject); err != nil {
				t.Fatal(err)
			}
			previous := append([]byte(nil), db.configuration.EncryptedConfig...)
			want := ErrForbidden
			var err error
			switch scenario {
			case "read denied":
				remote.mu.Lock()
				remote.listStatus = http.StatusForbidden
				remote.mu.Unlock()
				_, err = s.Configuration(context.Background(), user, agentAccessProject)
			case "write denied":
				remote.mu.Lock()
				remote.writeStatus = http.StatusForbidden
				remote.mu.Unlock()
				_, err = s.PutConfiguration(context.Background(), user, agentAccessProject, []byte(`[]`))
			case "local persistence failed":
				db.putError = errors.New("local database unavailable")
				want = ErrUnavailable
				_, err = s.PutConfiguration(context.Background(), user, agentAccessProject, []byte(`[]`))
			case "lock failed":
				db.lockError = errors.New("project lock unavailable")
				want = ErrUnavailable
				_, err = s.Configuration(context.Background(), user, agentAccessProject)
			}
			if !errors.Is(err, want) || !bytes.Equal(previous, db.configuration.EncryptedConfig) || db.locks != db.releases {
				t.Fatal("failed synchronization changed credentials or leaked the lock", err)
			}
			if scenario == "local persistence failed" {
				db.putError = nil
				if _, err := s.Configuration(context.Background(), user, agentAccessProject); err != nil {
					t.Fatal("reload could not repair a remote-only save", err)
				}
				loaded, err := s.agentCipher.Open(db.configuration)
				if err != nil || loaded.Value != "[]" || loaded.Version != 3 {
					t.Fatal("reload did not synchronize the remote-only version", err)
				}
			}
		})
	}
}

func TestConfigurationSyncKeepsUserAuthorizationAndLegacyFlow(t *testing.T) {
	t.Run("authorization precedes synchronization", func(t *testing.T) {
		s, db, remote, user := configurationSyncFixture(t)
		agent, _ := agentaccess.Generate()
		if _, err := s.Configuration(context.Background(), agent, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("agent key read configuration", err)
		}
		if _, err := s.PutConfiguration(context.Background(), agent, agentAccessProject, []byte(`[]`)); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("agent key changed configuration", err)
		}
		if _, err := s.Configuration(context.Background(), user, "1122334455"); !errors.Is(err, ErrNotFound) {
			t.Fatal("foreign project reached configuration", err)
		}
		if db.locks != 0 || db.puts != 0 || remote.reads != 0 || remote.writes != 0 {
			t.Fatal("unauthorized request reached synchronization")
		}
	})
	t.Run("local copy disabled", func(t *testing.T) {
		s, db, remote, user := configurationSyncFixture(t)
		s.agentDB, s.agentCipher = nil, nil
		remote.requireLock = false
		if document, err := s.Configuration(context.Background(), user, agentAccessProject); err != nil || document.Version != 2 {
			t.Fatal("legacy read failed", err)
		}
		if metadata, err := s.PutConfiguration(context.Background(), user, agentAccessProject, []byte(`[]`)); err != nil || metadata.Version != 3 {
			t.Fatal("legacy save failed", err)
		}
		if db.locks != 0 || db.puts != 0 || remote.writes != 1 {
			t.Fatal("disabled local copy changed legacy behavior")
		}
	})
}
