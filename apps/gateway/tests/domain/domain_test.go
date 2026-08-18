package domain_test

import (
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestEntityCanonicalizationPreservesCaseSensitiveValues(t *testing.T) {
	upper := domain.NewEntity("file", "Readme", domain.Provenance{})
	lower := domain.NewEntity("file", "README", domain.Provenance{})
	if upper.Value == lower.Value {
		t.Fatal("case-sensitive entity values were merged")
	}
}
