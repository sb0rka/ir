package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/ptnad"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

func TestFindingContextExpansionIsOptional(t *testing.T) {
	const rootID = "11111111-1111-4111-8111-111111111111"
	const childID = "22222222-2222-4222-8222-222222222222"
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for _, kind := range []string{"siem_incident", "siem_correlation", "nad_attack"} {
		for _, mode := range []string{"omitted", "false", "true"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				var rootCalls, childCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					body, _ := io.ReadAll(r.Body)
					switch {
					case r.URL.Path == "/api/incident_read_model/v1/incidents/"+rootID:
						rootCalls.Add(1)
						fmt.Fprintf(w, `{"id":%q,"name":"Incident","detectedAt":"2026-09-01T12:00:00Z","severity":"high"}`, rootID)
					case r.URL.Path == "/api/events/v3/events" && !strings.Contains(string(body), childID):
						rootCalls.Add(1)
						fmt.Fprintf(w, `{"totalCount":1,"events":[{"uuid":%q,"time":"2026-09-01T12:00:00Z","text":"Correlation","correlation_name":"Rule","subevents":[%q],"count.subevents":1}]}`, rootID, childID)
					case r.URL.Path == "/api/v2/bql":
						rootCalls.Add(1)
						fmt.Fprintf(w, `{"total":1,"result":[[null,null,"10.0.0.1","test",false,%q,"Attack",1,1,1,null,"2026-09-01T12:00:00Z",null,null,"10.0.0.2",[["tcp","10.0.0.2",80,"2026-09-01T12:00:00Z",[],false,%q,null,null,null,null,null,"10.0.0.1",123,"2026-09-01T12:00:00Z",[]]]]]}`, rootID, childID)
					default:
						childCalls.Add(1)
						http.Error(w, "child unavailable", http.StatusNotFound)
					}
				}))
				defer upstream.Close()
				var provider registry.Provider
				ref := domain.SourceObjectRef{RecordType: kind, ExternalID: rootID, TimeRange: domain.TimeRange{From: at.Add(-time.Hour), To: at.Add(time.Hour)}}
				if kind == "nad_attack" {
					client, err := ptnad.NewClient(ptnad.Config{BaseURL: upstream.URL, HTTPClient: upstream.Client()})
					if err != nil {
						t.Fatal(err)
					}
					adapter, err := ptnad.NewProvider(client, []int64{19})
					if err != nil {
						t.Fatal(err)
					}
					provider = adapter.RegistryProvider()
					ref.SourceInstance = "19"
				} else {
					config := proxy.HTTPClientConfig{BaseURL: upstream.URL, Timeout: time.Second}
					var err error
					provider, err = maxpatrol.NewProvider(maxpatrol.ClientConfig{SIEM: config, Incidents: config})
					if err != nil {
						t.Fatal(err)
					}
				}
				ref.SourceCode = provider.Source.Code
				service := New(aggregationRegistry(t, provider), &aggregationSecrets{}, time.Second, 5*time.Second)
				request := ResolveContextRequest{Findings: []domain.SourceObjectRef{ref}}
				if mode != "omitted" {
					value := mode == "true"
					request.ExpandFindings = &value
				}
				result, err := service.ResolveContext(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "test"}, request)
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Findings) != 1 || result.Findings[0].Ref.ExternalID != rootID {
					t.Fatalf("root lost: %+v", result)
				}
				if mode == "false" {
					if rootCalls.Load() != 1 || childCalls.Load() != 0 || len(result.Events)+len(result.Entities)+len(result.Sessions)+len(result.Relations) != 0 {
						t.Fatalf("expanded root: root calls=%d child calls=%d result=%+v", rootCalls.Load(), childCalls.Load(), result)
					}
					if len(result.Resolutions) != 1 || result.Resolutions[0].Status != "complete" || len(result.SourceErrors) != 0 {
						t.Fatalf("root-only selection incorrectly partial: %+v", result)
					}
				} else if childCalls.Load() == 0 {
					t.Fatal("default/true did not request child context")
				}
			})
		}
	}
}
