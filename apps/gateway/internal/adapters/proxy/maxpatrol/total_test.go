package maxpatrol

import "testing"

func TestAuthenticMatchTotal(t *testing.T) {
	if got := authenticMatchTotal(1523, 100); got == nil || *got != 1523 {
		t.Fatalf("reported total: got %#v", got)
	}
	if got := authenticMatchTotal(0, 0); got == nil || *got != 0 {
		t.Fatalf("empty page: got %#v", got)
	}
	if got := authenticMatchTotal(0, 50); got != nil {
		t.Fatalf("unknown noCount page must be nil, got %#v", got)
	}
}

func TestFindingsMatchTotal(t *testing.T) {
	inc := int64(10)
	corr := int64(5)
	if got := findingsMatchTotal(true, false, true, false, &inc, nil); got == nil || *got != 10 {
		t.Fatalf("incidents only: %#v", got)
	}
	if got := findingsMatchTotal(false, true, false, true, nil, &corr); got == nil || *got != 5 {
		t.Fatalf("correlations only: %#v", got)
	}
	if got := findingsMatchTotal(true, true, true, true, &inc, &corr); got == nil || *got != 15 {
		t.Fatalf("both kinds: %#v", got)
	}
	if got := findingsMatchTotal(true, true, true, false, &inc, nil); got != nil {
		t.Fatalf("both kinds incomplete must be nil, got %#v", got)
	}
}
