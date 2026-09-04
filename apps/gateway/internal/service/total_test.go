package service

import (
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestSumSourceTotals(t *testing.T) {
	a := int64(10)
	b := int64(20)
	got := sumSourceTotals([]domain.SourceState{
		{Source: "a", Status: "complete", Total: &a},
		{Source: "b", Status: "truncated", Total: &b},
	})
	if got == nil || *got != 30 {
		t.Fatalf("sum: %#v", got)
	}

	got = sumSourceTotals([]domain.SourceState{
		{Source: "a", Status: "complete", Total: &a},
		{Source: "b", Status: "complete"},
	})
	if got != nil {
		t.Fatalf("missing total must omit response total, got %#v", got)
	}

	got = sumSourceTotals([]domain.SourceState{
		{Source: "a", Status: "complete", Total: &a},
		{Source: "b", Status: "failed"},
	})
	if got == nil || *got != 10 {
		t.Fatalf("failed source ignored: %#v", got)
	}
}
