package maxpatrol

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/scenario"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestMockUsesMappedVendorPage(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	provider := NewMock(value)
	if len(provider.Source.Capabilities) != 1 || provider.Source.Capabilities[0] != domain.CapabilityEvents || provider.EntityLookup != nil {
		t.Fatalf("provider claims an unverified capability: %#v", provider.Source.Capabilities)
	}
	request := capability.SearchEventsRequest{Limit: 1}
	vendorRequest := toEventsRequest(request)
	if len(vendorRequest.Filter.Select) != 11 || vendorRequest.Filter.OrderBy[0].Field != "time" {
		t.Fatalf("unexpected vendor request: %#v", vendorRequest)
	}
	page, err := provider.Events.SearchEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || len(page.Continuations) != 1 || page.Events[0].Provenance.Source != SourceCode {
		t.Fatalf("unexpected first page: %#v", page)
	}
	repeated, err := provider.Events.SearchEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(page)
	right, _ := json.Marshal(repeated)
	if !bytes.Equal(left, right) {
		t.Fatal("repeated mock call is not byte-stable")
	}
	request.Cursor = page.Continuations[0]
	next, err := provider.Events.SearchEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Events) != 1 || next.Events[0].ID == page.Events[0].ID {
		t.Fatalf("cursor did not advance: %#v", next)
	}
}
