package domain

import "testing"

func TestCanonicalValueCollapsesAccountBackslashes(t *testing.T) {
	t.Parallel()

	got := CanonicalValue("account", `dkrylova\\administrator`)
	want := `dkrylova\administrator`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = CanonicalValue("account", `dkrylova\\\administrator`)
	if got != want {
		t.Fatalf("triple collapse: got %q want %q", got, want)
	}
	got = CanonicalValue("account", `dkrylova\administrator`)
	if got != want {
		t.Fatalf("single backslash must stay: got %q want %q", got, want)
	}
}
