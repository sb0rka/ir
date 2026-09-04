package server

import "testing"

func TestNormalizeRunTimeZone(t *testing.T) {
	t.Parallel()

	ok := "Europe/Moscow"
	got, err := normalizeRunTimeZone(&ok)
	if err != nil || got != ok {
		t.Fatalf("Europe/Moscow: got %q err=%v", got, err)
	}
	utc := "UTC"
	got, err = normalizeRunTimeZone(&utc)
	if err != nil || got != utc {
		t.Fatalf("UTC: got %q err=%v", got, err)
	}
	if got, err := normalizeRunTimeZone(nil); err != nil || got != "" {
		t.Fatalf("nil: got %q err=%v", got, err)
	}
	bad := "Москва"
	if _, err := normalizeRunTimeZone(&bad); err == nil {
		t.Fatal("Cyrillic label must be rejected")
	}
}
