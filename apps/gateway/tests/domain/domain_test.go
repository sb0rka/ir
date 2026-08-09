package domain_test

import (
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestStableIDPreservesCaseSensitiveParts(t *testing.T) {
	upper := domain.NewEntity("file", "Readme", domain.Provenance{})
	lower := domain.NewEntity("file", "README", domain.Provenance{})
	if upper.ID == lower.ID {
		t.Fatal("case-sensitive entity values produced the same stable ID")
	}
}
