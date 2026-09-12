package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/som"
)

// Project isolation for SOM update is the project-scoped DEMO_SOM_ACCESS_TOKEN
// cache key, not a store filter — there is no IR-owned SOM issue row.

func TestUpdateSomIssue(t *testing.T) {
	t.Parallel()

	projectID := "aabbccddee"
	issueID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	boardID := "22222222-2222-4222-8222-222222222222"
	desc := "updated description"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch || r.URL.Path != "/v1/issues/"+issueID.String() {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer cached-token" {
				t.Fatalf("authorization: %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			if strings.TrimSpace(string(body)) != `{"description":"updated description"}` {
				t.Fatalf("body: %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"id":           issueID.String(),
					"board_id":     boardID,
					"issue_number": 7,
					"simple_id":    "IR-7",
					"title":        "Task",
					"description":  desc,
				},
				"txid": 1,
			})
		}))
		defer upstream.Close()

		server := &Server{som: somclient.New(somclient.Config{APIBaseURL: upstream.URL})}
		server.somAuth.replace(projectID, "cached-token")
		ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: projectID})

		resp, err := server.UpdateSomIssue(ctx, som.UpdateSomIssueRequestObject{
			IssueId: issueID,
			Body:    &som.SomIssueUpdateRequest{Description: &desc},
		})
		if err != nil {
			t.Fatal(err)
		}
		ok, okType := resp.(som.UpdateSomIssue200JSONResponse)
		if !okType {
			t.Fatalf("response type: %T", resp)
		}
		if ok.Id != issueID || ok.Title != "Task" || ok.Description == nil || *ok.Description != desc {
			t.Fatalf("issue: %+v", ok)
		}
	})

	t.Run("nil body", func(t *testing.T) {
		t.Parallel()
		server := &Server{som: somclient.New(somclient.Config{APIBaseURL: "http://unused.example"})}
		server.somAuth.replace(projectID, "cached-token")
		ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: projectID})

		_, err := server.UpdateSomIssue(ctx, som.UpdateSomIssueRequestObject{IssueId: issueID})
		var domain *httperr.Error
		if !errors.As(err, &domain) || domain.Status != http.StatusBadRequest {
			t.Fatalf("want 400, got %#v", err)
		}
	})

	t.Run("som 404", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`issue not found`))
		}))
		defer upstream.Close()

		server := &Server{som: somclient.New(somclient.Config{APIBaseURL: upstream.URL})}
		server.somAuth.replace(projectID, "cached-token")
		ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: projectID})

		_, err := server.UpdateSomIssue(ctx, som.UpdateSomIssueRequestObject{
			IssueId: issueID,
			Body:    &som.SomIssueUpdateRequest{Description: &desc},
		})
		var domain *httperr.Error
		if !errors.As(err, &domain) || domain.Status != http.StatusNotFound {
			t.Fatalf("want 404, got %#v", err)
		}
	})

	t.Run("som 403 becomes 502", func(t *testing.T) {
		t.Parallel()
		calls := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`forbidden`))
		}))
		defer upstream.Close()

		server := &Server{som: somclient.New(somclient.Config{APIBaseURL: upstream.URL})}
		server.somAuth.replace(projectID, "cached-token")
		// No secrets client: after auth retry reload, somBearer fails with
		// source_unavailable — same 502 class as a mapped upstream 403.
		ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: projectID})

		_, err := server.UpdateSomIssue(ctx, som.UpdateSomIssueRequestObject{
			IssueId: issueID,
			Body:    &som.SomIssueUpdateRequest{Description: &desc},
		})
		var domain *httperr.Error
		if !errors.As(err, &domain) || domain.Status != http.StatusBadGateway {
			t.Fatalf("want 502, got %#v", err)
		}
		if calls < 1 {
			t.Fatal("expected at least one upstream call")
		}
	})
}
