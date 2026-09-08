package ptnad

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMailDateFromLiveHeaders(t *testing.T) {
	for _, raw := range []string{
		`{"id":"mail-1","from":"attacker@mail.ru","to":["victim@ptpilot.local"],"headers":[{"key":"To","value":["victim@ptpilot.local"]},{"key":"Date","value":"10 Jun 2023 12:57:52 +0300"}]}`,
		`{"headers.key":["Date"],"headers.value":["10 Jun 2023 12:57:52 +0300"]}`,
		`{"date":"10 Jun 2023 12:57:52 +0300","headers":[{"key":"Date","value":"different"}]}`,
	} {
		var hint MailHint
		if err := json.Unmarshal([]byte(raw), &hint); err != nil {
			t.Fatal(err)
		}
		if hint.Date != "10 Jun 2023 12:57:52 +0300" {
			t.Fatalf("date lost: %+v", hint)
		}
		event := mailEvent(Session{}, hint, 0)
		if !event.OccurredAt.Equal(time.Date(2023, 6, 10, 9, 57, 52, 0, time.UTC)) {
			t.Fatal(event.OccurredAt)
		}
		out, _ := json.Marshal(hint)
		if strings.Contains(string(out), "headers") {
			t.Fatal("raw MIME headers leaked")
		}
	}
}

func TestQuotedMailAccount(t *testing.T) {
	for _, value := range []string{"victim@ptpilot.local", `"victim@ptpilot.local"`} {
		if got := normalizeAccount(value); got != "victim@ptpilot.local" {
			t.Fatalf("quoted IMAP account: %q", got)
		}
	}
	if got := normalizeAccount(`"[redacted]"`); got != "" {
		t.Fatalf("redacted account retained: %q", got)
	}
}
