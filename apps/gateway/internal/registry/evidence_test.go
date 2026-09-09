package registry

import (
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestEvidenceCapabilityRequiresImplementation(t *testing.T) {
	for _, kind := range []domain.Capability{domain.CapabilityEvidencePayload, domain.CapabilityEvidenceFile} {
		_, err := New(Provider{Source: domain.Source{Code: "nad", Capabilities: []domain.Capability{kind}}, CredentialSecret: "cookie"})
		if err == nil {
			t.Fatalf("advertised %s without an implementation", kind)
		}
	}
}
