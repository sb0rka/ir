package ptnad

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (client *Client) attackDetail(ctx context.Context, ref AttackRef, parent string, access Access) (alertDetail, error) {
	if _, _, err := validateAttackRef(ref); err != nil {
		return alertDetail{}, err
	}
	if err := validateExternalID(parent); err != nil {
		return alertDetail{}, err
	}
	query := fmt.Sprintf("start=%d&end=%d&source=%d", ref.TimeRange.From.UnixMilli(), ref.TimeRange.To.UnixMilli(), ref.StoreID)
	var detail alertDetail
	if err := client.doJSON(ctx, "attack detail", http.MethodGet, "api/v2/flow/"+parent+"/alert/"+ref.ExternalID, query, "", access, &detail); err != nil {
		return detail, err
	}
	if detail.ID != ref.ExternalID || detail.Parent != parent {
		return alertDetail{}, &ProtocolError{Operation: "attack detail identity"}
	}
	return detail, nil
}

func (client *Client) enrichAttack(ctx context.Context, found Attack, ref AttackRef, access Access) Attack {
	if found.ParentSession == nil {
		found.ContextErrors = append(found.ContextErrors, fmt.Errorf("parent session unavailable"))
		return found
	}
	detail, err := client.attackDetail(ctx, ref, found.ParentSession.ExternalID, access)
	if err != nil {
		found.ContextErrors = append(found.ContextErrors, err)
		return found
	}
	if detail.Timestamp == "" {
		detail.Timestamp = found.OccurredAt.Format(time.RFC3339Nano)
	}
	enriched, err := mapAttackDetail(detail, ref.StoreID, ref.TimeRange, found.FetchedAt)
	if err != nil {
		found.ContextErrors = append(found.ContextErrors, err)
		return found
	}
	return enriched
}

func mailEvent(session Session, value MailHint, index int) domain.Event {
	mentions := append([]domain.EntityMention(nil), sessionEndpointMentions(session)...)
	sender := mailAddress(value.From)
	if sender != "" {
		mentions = append(mentions, mention("email", sender, "src"))
	}
	recipients := []string{}
	for _, raw := range value.To {
		if address := mailAddress(raw); address != "" {
			recipients = append(recipients, address)
			mentions = append(mentions, mention("email", address, "dst"))
		}
	}
	occurred := session.Start
	if date, err := mail.ParseDate(value.Date); err == nil {
		occurred = date.UTC()
	}
	// Source IDs preserve stable identity when available; index is scoped to the
	// parent session when NAD's mail record has no independent identifier.
	id := firstNonEmpty(value.ID, strconv.Itoa(index))
	return domain.Event{Type: "network.mail", Title: firstNonEmpty(safeText(value.Subject), "Mail message"), Severity: "info", OccurredAt: occurred,
		Entities: normalizeMentions(mentions), Attributes: map[string]any{"parent_session_id": session.SourceRef.ExternalID, "from": sender, "to": recipients, "subject": safeText(value.Subject), "date": safeText(value.Date)},
		Provenance: childProvenance(session, "nad_mail", id)}
}

func mailAddress(value string) string {
	address, err := mail.ParseAddress(value)
	if err != nil || len(address.Address) > 320 {
		return ""
	}
	return strings.ToLower(address.Address)
}
