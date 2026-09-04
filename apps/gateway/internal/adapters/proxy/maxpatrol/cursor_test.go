package maxpatrol

import "testing"

func TestOffsetContinuation(t *testing.T) {
	total := int64(250)
	if next, more := offsetContinuation(0, 100, 100, &total); !more || next != 100 {
		t.Fatalf("first page of 250: next=%d more=%v", next, more)
	}
	if next, more := offsetContinuation(200, 50, 100, &total); more || next != 250 {
		t.Fatalf("last page must end: next=%d more=%v", next, more)
	}
	exact := int64(100)
	if _, more := offsetContinuation(0, 100, 100, &exact); more {
		t.Fatal("total reached must not continue")
	}
	// noCount pages carry no total: a full page continues, a short page ends.
	if next, more := offsetContinuation(100, 100, 100, nil); !more || next != 200 {
		t.Fatalf("full noCount page: next=%d more=%v", next, more)
	}
	if _, more := offsetContinuation(100, 40, 100, nil); more {
		t.Fatal("short noCount page must end")
	}
	if _, more := offsetContinuation(300, 0, 100, nil); more {
		t.Fatal("empty page must end")
	}
}

func TestEventCursorRoundTrip(t *testing.T) {
	encoded, err := encodeEventCursor(eventCursor{Offset: 300})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeEventCursor(encoded)
	if err != nil || decoded.Offset != 300 {
		t.Fatalf("decode: %#v %v", decoded, err)
	}
	if empty, err := decodeEventCursor(" "); err != nil || empty.Offset != 0 {
		t.Fatalf("blank cursor must start at 0: %#v %v", empty, err)
	}
	if _, err := decodeEventCursor("not-base64!"); err == nil {
		t.Fatal("garbage cursor must be rejected")
	}
	negative, _ := encodeEventCursor(eventCursor{Offset: -1})
	if _, err := decodeEventCursor(negative); err == nil {
		t.Fatal("negative offset must be rejected")
	}
}

func TestFindingCursorKeepsCorrelationOffset(t *testing.T) {
	encoded, err := encodeFindingCursor(findingCursor{IncidentDone: true, CorrelationOffset: 100})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeFindingCursor(encoded)
	if err != nil || !decoded.IncidentDone || decoded.CorrelationOffset != 100 || decoded.CorrelationsDone {
		t.Fatalf("decode: %#v %v", decoded, err)
	}
}
