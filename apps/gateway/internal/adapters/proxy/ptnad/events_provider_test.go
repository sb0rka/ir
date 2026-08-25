package ptnad

import (
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

func TestHasSourceEventControls(t *testing.T) {
	if hasSourceEventControls(capability.SearchEventsRequest{}) {
		t.Fatal("empty request must remain compatible")
	}
	if !hasSourceEventControls(capability.SearchEventsRequest{Columns: []string{"time"}}) {
		t.Fatal("PT NAD must reject unsupported source controls")
	}
}
