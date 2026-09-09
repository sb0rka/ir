package ptnad

import (
	"testing"
	"time"
)

func TestFullMailFilesAndPartialContext(t *testing.T) {
	var detail flowDetail
	fixture(t, "imap-session.json", &detail)
	session, err := mapFlowDetail(detail, 23, caseWindow(), time.Now())
	if err != nil || len(session.Files) != 5 || len(session.ContextErrors) != 0 {
		t.Fatalf("full mail: files=%d errors=%v err=%v", len(session.Files), session.ContextErrors, err)
	}
	// A foreign child is excluded without losing the mail, account, or root.
	detail.Files[0].Parent = "different-session"
	session, err = mapFlowDetail(detail, 23, caseWindow(), time.Now())
	if err != nil || session.SourceRef.ExternalID != detail.ID || len(session.Files) != 4 || len(session.ContextErrors) != 1 || len(session.Mail) == 0 || len(session.Authentication) == 0 {
		t.Fatalf("partial mail: %+v, err=%v", session, err)
	}
	for _, file := range session.Files {
		if file.ExternalID == detail.Files[0].ID {
			t.Fatal("foreign file was retained")
		}
	}
}
