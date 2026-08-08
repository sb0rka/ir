package mock

import "testing"

func TestRegistryIncludesOnlyContractBackedProviders(t *testing.T) {
	providers, _, err := NewRegistry(Options{EventCount: 1, EndpointCount: 1, HistoryDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	sources := providers.Sources()
	if len(sources) != 2 || sources[0].Code != "maxpatrol-siem" || sources[1].Code != "pt-sandbox" {
		t.Fatalf("unexpected registered providers: %#v", sources)
	}
}
