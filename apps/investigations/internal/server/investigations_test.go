package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

type updateRecordingDB struct {
	store.Database
	patch model.InvestigationPatch
}

type deleteInvestigationRecordingDB struct {
	store.Database
	projectID       string
	investigationID string
	err             error
}

func (db *deleteInvestigationRecordingDB) DeleteInvestigation(_ context.Context, projectID, investigationID string) error {
	db.projectID = projectID
	db.investigationID = investigationID
	return db.err
}

func (db *updateRecordingDB) UpdateInvestigation(_ context.Context, patch model.InvestigationPatch) (model.Investigation, error) {
	db.patch = patch
	return model.Investigation{
		ID: patch.InvestigationID, ProjectID: patch.ProjectID, Title: "case",
		Status: *patch.Status, Origin: "analyst", Version: patch.Version + 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}, nil
}

func TestUpdateInvestigationForwardsInProgressStatus(t *testing.T) {
	id := uuid.New()
	status := investigations.InProgress
	body := investigations.InvestigationPatch{Version: 3, Status: &status}
	db := &updateRecordingDB{}
	server := &Server{db: db}
	ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: "project"})

	response, err := server.UpdateInvestigation(ctx, investigations.UpdateInvestigationRequestObject{
		InvestigationId: id,
		Body:            &body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(investigations.UpdateInvestigation200JSONResponse); !ok {
		t.Fatalf("unexpected response %T", response)
	}
	if db.patch.Status == nil || *db.patch.Status != "in_progress" || db.patch.Version != 3 {
		t.Fatalf("unexpected patch: %+v", db.patch)
	}
}

func TestDeleteInvestigation(t *testing.T) {
	id := uuid.New()
	ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: "aabbccddee"})

	t.Run("success", func(t *testing.T) {
		db := &deleteInvestigationRecordingDB{}
		server := &Server{db: db}
		response, err := server.DeleteInvestigation(ctx, investigations.DeleteInvestigationRequestObject{InvestigationId: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := response.(investigations.DeleteInvestigation204Response); !ok {
			t.Fatalf("unexpected response %T", response)
		}
		if db.projectID != "aabbccddee" || db.investigationID != id.String() {
			t.Fatalf("delete scope project=%q investigation=%q", db.projectID, db.investigationID)
		}
	})

	t.Run("missing or foreign project", func(t *testing.T) {
		db := &deleteInvestigationRecordingDB{err: store.ErrInvestigationNotFound}
		server := &Server{db: db}
		_, err := server.DeleteInvestigation(ctx, investigations.DeleteInvestigationRequestObject{InvestigationId: id})
		var domain *httperr.Error
		if !errorsAs(err, &domain) || domain.Status != http.StatusNotFound {
			t.Fatalf("delete error=%v", err)
		}
	})

	t.Run("missing project scope", func(t *testing.T) {
		db := &deleteInvestigationRecordingDB{}
		server := &Server{db: db}
		_, err := server.DeleteInvestigation(context.Background(), investigations.DeleteInvestigationRequestObject{InvestigationId: id})
		var domain *httperr.Error
		if !errorsAs(err, &domain) || domain.Status != http.StatusBadRequest {
			t.Fatalf("delete error=%v", err)
		}
		if db.investigationID != "" {
			t.Fatal("store called without project scope")
		}
	})

	t.Run("invalid investigation id", func(t *testing.T) {
		db := &deleteInvestigationRecordingDB{}
		server := &Server{db: db}
		handler := investigations.Handler(investigations.NewStrictHandler(server, nil))
		request := httptest.NewRequest(http.MethodDelete, "/investigations/not-a-uuid", nil)
		request.Header.Set("X-Project-ID", "aabbccddee")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", response.Code)
		}
		if db.investigationID != "" {
			t.Fatal("store called for invalid investigation id")
		}
	})
}

func TestConvertEventSummaryPreservesSeedMarker(t *testing.T) {
	converted, err := convertEventSummary(model.EventSummary{
		ID: uuid.NewString(), SourceCode: "siem", SourceEventID: "event-1",
		Title: "seed", EventType: "alert", OccurredAt: time.Now(), IngestedAt: time.Now(),
		AttachedAt: time.Now(), AttachedBy: "analyst", IsSeed: true, NormalizedData: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !converted.IsSeed || converted.AttachedBy == nil || *converted.AttachedBy != events.Analyst {
		t.Fatalf("unexpected event: %+v", converted)
	}
}
