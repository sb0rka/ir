package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

type updateRecordingDB struct {
	store.Database
	patch model.InvestigationPatch
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
