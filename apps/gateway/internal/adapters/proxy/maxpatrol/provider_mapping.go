package maxpatrol

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func findingFromIncident(value Incident, timeRange domain.TimeRange, fetchedAt time.Time, entities []domain.EntityMention) domain.Finding {
	changedAt := value.ChangedAt
	if changedAt.IsZero() {
		changedAt = time.Time{}
	}
	var changedAtPointer *time.Time
	if !changedAt.IsZero() {
		changedAtPointer = &changedAt
	}
	assignedTo := ""
	if value.AssignedTo != nil {
		assignedTo = firstNonEmpty(value.AssignedTo.Name, value.AssignedTo.ID)
	}
	return domain.Finding{
		Ref: domain.SourceObjectRef{
			SourceCode: SourceCode, RecordType: IncidentRecordType, ExternalID: value.ID, TimeRange: utcTimeRange(timeRange),
		},
		Kind:        IncidentRecordType,
		Title:       firstNonEmpty(value.Name, value.Key, value.ID),
		Description: value.Description,
		Severity:    value.Severity,
		OccurredAt:  value.DetectedAt.UTC(),
		Status:      strings.ToLower(strings.TrimSpace(value.State)),
		Rule:        &domain.RuleRef{Name: value.Name},
		Entities:    dedupeDomainMentions(entities),
		Incident: &domain.IncidentDetails{
			Key:            value.Key,
			ExternalKey:    value.ExternalKey,
			Verdict:        strings.ToLower(value.Verdict),
			Damage:         strings.ToLower(value.Damage),
			Recommendation: value.Recommendation,
			AssignedTo:     assignedTo,
			ChangedAt:      changedAtPointer,
			Archived:       value.IsArchived,
			Removed:        value.IsRemoved,
		},
		RelatedFindings: []domain.SourceObjectRef{},
		RelatedSessions: []domain.SourceObjectRef{},
		SourceRef:       objectURN(IncidentRecordType, value.ID),
		FetchedAt:       fetchedAt,
	}
}

func findingFromCorrelation(value Correlation, timeRange domain.TimeRange, fetchedAt time.Time) domain.Finding {
	return domain.Finding{
		Ref: domain.SourceObjectRef{
			SourceCode: SourceCode, RecordType: CorrelationRecordType, ExternalID: value.UUID, TimeRange: utcTimeRange(timeRange),
		},
		Kind:            CorrelationRecordType,
		Title:           firstNonEmpty(value.Title, value.RuleName, value.UUID),
		Description:     value.Title,
		Severity:        value.Severity,
		OccurredAt:      value.OccurredAt.UTC(),
		Status:          "",
		Rule:            &domain.RuleRef{Name: value.RuleName},
		Entities:        domainMentions(value.Entities),
		Correlation:     &domain.CorrelationDetails{CorrelationType: strings.ToLower(value.CorrelationType), SubeventCount: value.SubeventCount},
		RelatedFindings: []domain.SourceObjectRef{},
		RelatedSessions: []domain.SourceObjectRef{},
		SourceRef:       objectURN(CorrelationRecordType, value.UUID),
		FetchedAt:       fetchedAt,
	}
}

func (provider *Provider) incidentContext(timeRange domain.TimeRange, value IncidentResolution) capability.ContextPage {
	fetchedAt := provider.client.now().UTC()
	page := capability.ContextPage{}
	rootMentions, rootEntities, rootRelations := incidentEntities(value, fetchedAt)
	root := findingFromIncident(value.Incident, timeRange, fetchedAt, rootMentions)
	page.Entities = append(page.Entities, rootEntities...)
	page.Relations = append(page.Relations, rootRelations...)

	for _, child := range value.Correlations {
		correlationPage := provider.correlationContext(timeRange, child)
		childFinding := correlationPage.Findings[0]
		root.RelatedFindings = append(root.RelatedFindings, childFinding.Ref)
		page.Findings = append(page.Findings, childFinding)
		page.Events = append(page.Events, correlationPage.Events...)
		page.Entities = append(page.Entities, correlationPage.Entities...)
		page.Relations = append(page.Relations, correlationPage.Relations...)
		page.Resolutions = append(page.Resolutions, correlationPage.Resolutions...)
		root.Entities = append(root.Entities, childFinding.Entities...)
		for _, event := range correlationPage.Events {
			root.Entities = append(root.Entities, event.Entities...)
		}
	}
	for _, raw := range value.Events {
		event, entities, relations := domainEventFromRaw(raw, fetchedAt, "")
		event.Attributes["parent_finding_id"] = value.Incident.ID
		page.Events = append(page.Events, event)
		page.Entities = append(page.Entities, entities...)
		page.Relations = append(page.Relations, relations...)
		root.Entities = append(root.Entities, event.Entities...)
	}
	metadataEvents, metadataEntities := incidentMetadataContext(value, fetchedAt)
	page.Events = append(page.Events, metadataEvents...)
	page.Entities = append(page.Entities, metadataEntities...)
	for _, event := range metadataEvents {
		root.Entities = append(root.Entities, event.Entities...)
	}
	root.Entities = dedupeDomainMentions(root.Entities)
	root.RelatedFindings = dedupeObjectRefs(root.RelatedFindings)
	page.Findings = append([]domain.Finding{root}, page.Findings...)
	rootErrors := append([]ContextError(nil), value.Errors...)
	if value.Truncated {
		rootErrors = append(rootErrors, ContextError{Component: "incident.context", Code: "truncated", Message: "incident context exceeded a safety limit"})
	}
	page.Resolutions = append(page.Resolutions, objectResolution(root.Ref, value.Complete && !value.Truncated, rootErrors))
	return page
}

func incidentMetadataContext(value IncidentResolution, fetchedAt time.Time) ([]domain.Event, []domain.Entity) {
	events := make([]domain.Event, 0, len(value.Files)+len(value.Links)+len(value.AssetGroups))
	entities := make([]domain.Entity, 0, len(value.Files)*3+len(value.Links))
	appendEvent := func(kind, key, title string, attributes map[string]any, mentions []domain.EntityMention) {
		attributes["parent_finding_id"] = value.Incident.ID
		externalID := value.Incident.ID + ":" + kind + ":" + safeMetadataID(key)
		event := domain.Event{
			Type: kind, Title: title, Severity: value.Incident.Severity, OccurredAt: value.Incident.DetectedAt.UTC(),
			Entities: dedupeDomainMentions(mentions), Attributes: attributes,
			Provenance: domain.Provenance{Source: SourceCode, ExternalID: externalID, SourceURL: objectURN(IncidentRecordType, value.Incident.ID), FetchedAt: fetchedAt},
		}
		events = append(events, event)
		entities = append(entities, entitiesFromMentions(event.Entities, fetchedAt)...)
	}
	for _, file := range value.Files {
		mentions := make([]domain.EntityMention, 0, 3)
		for _, hash := range []string{file.MD5, file.SHA1, file.SHA256} {
			if hash == "" {
				continue
			}
			mentions = append(mentions, domain.EntityMention{
				EntityRef: domain.EntityRef{Type: "hash", Value: domain.CanonicalValue("hash", hash)}, Roles: []string{"object"},
			})
		}
		appendEvent("siem.incident.file", firstNonEmpty(file.ID, file.SHA256, file.SHA1, file.MD5, file.Path+"\x00"+file.Name),
			firstNonEmpty(file.Name, "Incident file"), map[string]any{
				"name": file.Name, "path": file.Path, "size": file.Size,
			}, mentions)
	}
	for _, link := range value.Links {
		mentions := []domain.EntityMention{}
		if link.URL != "" {
			mentions = append(mentions, domain.EntityMention{
				EntityRef: domain.EntityRef{Type: "url", Value: domain.CanonicalValue("url", link.URL)}, Roles: []string{"object"},
			})
		}
		appendEvent("siem.incident.link", link.Name+"\x00"+link.URL, firstNonEmpty(link.Name, "Incident link"),
			map[string]any{"name": link.Name, "internal": link.Internal}, mentions)
	}
	for _, group := range value.AssetGroups {
		appendEvent("siem.incident.asset_group", firstNonEmpty(group.ID, group.Name), firstNonEmpty(group.Name, "Incident asset group"),
			map[string]any{"group_id": group.ID, "name": group.Name, "description": group.Description}, nil)
	}
	return events, entities
}

func safeMetadataID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

func (provider *Provider) correlationContext(timeRange domain.TimeRange, value CorrelationResolution) capability.ContextPage {
	fetchedAt := provider.client.now().UTC()
	finding := findingFromCorrelation(value.Correlation, timeRange, fetchedAt)
	rootEvent, rootEntities, rootRelations := domainEventFromCorrelation(value.Correlation, fetchedAt)
	page := capability.ContextPage{
		Findings:  []domain.Finding{finding},
		Events:    []domain.Event{rootEvent},
		Entities:  rootEntities,
		Relations: rootRelations,
	}
	for _, raw := range value.Subevents {
		event, entities, relations := domainEventFromRaw(raw, fetchedAt, value.Correlation.UUID)
		page.Events = append(page.Events, event)
		page.Entities = append(page.Entities, entities...)
		page.Relations = append(page.Relations, relations...)
		page.Findings[0].Entities = append(page.Findings[0].Entities, event.Entities...)
	}
	page.Findings[0].Entities = dedupeDomainMentions(page.Findings[0].Entities)
	page.Resolutions = []domain.ObjectResolution{objectResolution(finding.Ref, value.Complete, value.Errors)}
	return page
}

func domainEventFromRecord(record safeEventRecord, fetchedAt time.Time, parentCorrelationID string) (domain.Event, []domain.Entity, []domain.Relation, error) {
	raw, err := eventToRaw(record)
	if err != nil {
		return domain.Event{}, nil, nil, err
	}
	event, entities, relations := domainEventFromRaw(raw, fetchedAt, parentCorrelationID)
	if strings.TrimSpace(record.CorrelationName) != "" {
		event.Type = CorrelationRecordType
		event.Attributes["correlation_name"] = cleanText(record.CorrelationName, maxNameLength)
		event.Attributes["correlation_type"] = cleanText(record.CorrelationType, maxNameLength)
		event.Attributes["count.subevents"] = record.SubeventCount
	}
	return event, entities, relations, nil
}

func domainEventFromCorrelation(value Correlation, fetchedAt time.Time) (domain.Event, []domain.Entity, []domain.Relation) {
	mentions := domainMentions(value.Entities)
	event := domain.Event{
		Type:       CorrelationRecordType,
		Title:      value.Title,
		Severity:   value.Severity,
		OccurredAt: value.OccurredAt.UTC(),
		Entities:   mentions,
		Attributes: map[string]any{
			"action":           value.Action,
			"correlation_name": value.RuleName,
			"correlation_type": value.CorrelationType,
			"count.subevents":  value.SubeventCount,
		},
		Provenance: domain.Provenance{Source: SourceCode, ExternalID: value.UUID, SourceURL: objectURN(CorrelationRecordType, value.UUID), FetchedAt: fetchedAt},
	}
	entities := entitiesFromMentions(mentions, fetchedAt)
	return event, entities, relationsFromMentions(event, fetchedAt)
}

func domainEventFromRaw(value RawEvent, fetchedAt time.Time, parentCorrelationID string) (domain.Event, []domain.Entity, []domain.Relation) {
	attributes := map[string]any{}
	putSafeAttribute(attributes, "action", value.Action)
	putSafeAttribute(attributes, "event_src.host", value.EventSourceHost)
	putSafeAttribute(attributes, "event_src.ip", value.EventSourceIP)
	putSafeAttribute(attributes, "src.host", value.SourceHost)
	putSafeAttribute(attributes, "src.ip", value.SourceIP)
	if value.SourcePort > 0 {
		attributes["src.port"] = value.SourcePort
	}
	putSafeAttribute(attributes, "dst.host", value.DestinationHost)
	putSafeAttribute(attributes, "dst.ip", value.DestinationIP)
	if value.DestinationPort > 0 {
		attributes["dst.port"] = value.DestinationPort
	}
	putSafeAttribute(attributes, "actor.account", value.ActorAccount)
	putSafeAttribute(attributes, "object.account", value.ObjectAccount)
	putSafeAttribute(attributes, "subject.process.name", value.SubjectProcess.Name)
	putSafeAttribute(attributes, "subject.process.fullpath", value.SubjectProcess.Path)
	putSafeAttribute(attributes, "subject.process.cmdline", value.SubjectProcess.CommandLine)
	putSafeAttribute(attributes, "object.process.name", value.ObjectProcess.Name)
	putSafeAttribute(attributes, "object.process.fullpath", value.ObjectProcess.Path)
	putSafeAttribute(attributes, "object.process.cmdline", value.ObjectProcess.CommandLine)
	putSafeAttribute(attributes, "object.name", value.ObjectName)
	putSafeAttribute(attributes, "object.path", value.ObjectPath)
	putSafeAttribute(attributes, "category.generic", value.CategoryGeneric)
	putSafeAttribute(attributes, "category.high", value.CategoryHigh)
	putSafeAttribute(attributes, "category.low", value.CategoryLow)
	if parentCorrelationID != "" {
		attributes["parent_source_event_id"] = parentCorrelationID
		attributes["relation_type"] = "subevent_of"
	}
	mentions := domainMentions(value.Entities)
	event := domain.Event{
		Type:       "siem_event",
		Title:      value.Title,
		Severity:   value.Severity,
		OccurredAt: value.OccurredAt.UTC(),
		Entities:   mentions,
		Attributes: attributes,
		Provenance: domain.Provenance{Source: SourceCode, ExternalID: value.UUID, SourceURL: objectURN("siem_event", value.UUID), FetchedAt: fetchedAt},
	}
	entities := entitiesFromMentions(mentions, fetchedAt)
	relations := relationsFromMentions(event, fetchedAt)
	relations = append(relations, interfaceRelations(event, value, fetchedAt)...)
	return event, entities, relations
}

func incidentEntities(value IncidentResolution, fetchedAt time.Time) ([]domain.EntityMention, []domain.Entity, []domain.Relation) {
	mentions := make([]domain.EntityMention, 0, len(value.Hosts)*2+len(value.Accounts))
	entities := make([]domain.Entity, 0, cap(mentions))
	relations := make([]domain.Relation, 0, len(value.Hosts))
	for _, host := range value.Hosts {
		role := incidentHostRole(host.Role)
		var hostRef, ipRef domain.EntityRef
		if host.FQDN != "" {
			hostRef = domain.EntityRef{Type: "host", Value: domain.CanonicalValue("host", host.FQDN)}
			mentions = append(mentions, domain.EntityMention{EntityRef: hostRef, Roles: []string{role}})
			entities = append(entities, sourceEntity(hostRef, "host:"+hostRef.Value, fetchedAt))
		}
		if host.IP != nil && *host.IP != "" {
			ipRef = domain.EntityRef{Type: "ip", Value: domain.CanonicalValue("ip", *host.IP)}
			mentions = append(mentions, domain.EntityMention{EntityRef: ipRef, Roles: []string{role}})
			entities = append(entities, sourceEntity(ipRef, "ip:"+ipRef.Value, fetchedAt))
		}
		if hostRef.Value != "" && ipRef.Value != "" {
			relations = append(relations, domain.Relation{
				Type: "has_interface", SourceEntity: hostRef, TargetEntity: ipRef,
				Provenance: domain.Provenance{Source: SourceCode, ExternalID: value.Incident.ID + ":has_interface:" + hostRef.Value + ":" + ipRef.Value, SourceURL: objectURN(IncidentRecordType, value.Incident.ID), FetchedAt: fetchedAt},
			})
		}
	}
	for _, account := range value.Accounts {
		if account.Name == "" {
			continue
		}
		ref := domain.EntityRef{Type: "account", Value: domain.CanonicalValue("account", account.Name)}
		mentions = append(mentions, domain.EntityMention{EntityRef: ref, Roles: []string{"mentions"}})
		entities = append(entities, sourceEntity(ref, "account:"+ref.Value, fetchedAt))
	}
	return dedupeDomainMentions(mentions), entities, relations
}

func incidentHostRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "impactsource":
		return "src"
	case "impacttarget":
		return "dst"
	default:
		return "mentions"
	}
}

func domainMentions(values []EntityMention) []domain.EntityMention {
	result := make([]domain.EntityMention, 0, len(values))
	for _, value := range values {
		if value.Type == "" || value.Value == "" {
			continue
		}
		result = append(result, domain.EntityMention{
			EntityRef: domain.EntityRef{Type: value.Type, Value: domain.CanonicalValue(value.Type, value.Value)},
			Roles:     []string{value.Role},
		})
	}
	return dedupeDomainMentions(result)
}

func dedupeDomainMentions(values []domain.EntityMention) []domain.EntityMention {
	type mentionValue struct {
		ref   domain.EntityRef
		roles map[string]struct{}
	}
	seen := make(map[string]mentionValue, len(values))
	for _, value := range values {
		kind := strings.ToLower(strings.TrimSpace(value.Type))
		canonical := domain.CanonicalValue(kind, value.Value)
		if kind == "" || canonical == "" {
			continue
		}
		key := kind + "\x00" + canonical
		current, exists := seen[key]
		if !exists {
			current = mentionValue{ref: domain.EntityRef{Type: kind, Value: canonical}, roles: map[string]struct{}{}}
		}
		for _, role := range value.Roles {
			if role = strings.TrimSpace(role); role != "" {
				current.roles[role] = struct{}{}
			}
		}
		seen[key] = current
	}
	result := make([]domain.EntityMention, 0, len(seen))
	for _, value := range seen {
		roles := make([]string, 0, len(value.roles))
		for role := range value.roles {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		result = append(result, domain.EntityMention{EntityRef: value.ref, Roles: roles})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Type != result[right].Type {
			return result[left].Type < result[right].Type
		}
		return result[left].Value < result[right].Value
	})
	return result
}

func entitiesFromMentions(values []domain.EntityMention, fetchedAt time.Time) []domain.Entity {
	result := make([]domain.Entity, 0, len(values))
	for _, mention := range values {
		result = append(result, sourceEntity(mention.EntityRef, mention.Type+":"+mention.Value, fetchedAt))
	}
	return result
}

func sourceEntity(ref domain.EntityRef, externalID string, fetchedAt time.Time) domain.Entity {
	return domain.NewEntity(ref.Type, ref.Value, domain.Provenance{Source: SourceCode, ExternalID: externalID, FetchedAt: fetchedAt})
}

func interfaceRelations(event domain.Event, raw RawEvent, fetchedAt time.Time) []domain.Relation {
	pairs := [][2]string{
		{raw.EventSourceHost, raw.EventSourceIP},
		{raw.SourceHost, raw.SourceIP},
		{raw.DestinationHost, raw.DestinationIP},
	}
	result := make([]domain.Relation, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	for _, pair := range pairs {
		host := domain.CanonicalValue("host", pair[0])
		ip := domain.CanonicalValue("ip", pair[1])
		if host == "" || ip == "" {
			continue
		}
		key := host + "\x00" + ip
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, eventRelation(event, "has_interface", domain.EntityRef{Type: "host", Value: host}, domain.EntityRef{Type: "ip", Value: ip}, fetchedAt))
	}
	return result
}

func relationsFromMentions(event domain.Event, fetchedAt time.Time) []domain.Relation {
	sources := roleRefs(event.Entities, "src")
	destinations := roleRefs(event.Entities, "dst")
	actors := roleRefs(event.Entities, "actor")
	result := make([]domain.Relation, 0, len(sources)*len(destinations)+len(actors)*len(destinations))
	for _, source := range sources {
		for _, destination := range destinations {
			if source == destination {
				continue
			}
			result = append(result, eventRelation(event, "connected_to", source, destination, fetchedAt))
		}
	}
	for _, actor := range actors {
		for _, destination := range destinations {
			if destination.Type != "host" && destination.Type != "ip" {
				continue
			}
			result = append(result, eventRelation(event, "authenticated_to", actor, destination, fetchedAt))
		}
	}
	return result
}

func roleRefs(values []domain.EntityMention, role string) []domain.EntityRef {
	result := make([]domain.EntityRef, 0)
	for _, value := range values {
		for _, candidate := range value.Roles {
			if candidate == role {
				result = append(result, value.EntityRef)
				break
			}
		}
	}
	return result
}

func eventRelation(event domain.Event, relationType string, source, destination domain.EntityRef, fetchedAt time.Time) domain.Relation {
	return domain.Relation{
		Type: relationType, SourceEntity: source, TargetEntity: destination, OccurredAt: &event.OccurredAt,
		Provenance: domain.Provenance{
			Source: SourceCode, ExternalID: event.Provenance.ExternalID + ":" + relationType + ":" + source.Type + ":" + source.Value + ":" + destination.Type + ":" + destination.Value,
			SourceURL: event.Provenance.SourceURL, FetchedAt: fetchedAt,
		},
	}
}

func objectResolution(ref domain.SourceObjectRef, complete bool, errors []ContextError) domain.ObjectResolution {
	status := "complete"
	if !complete || len(errors) > 0 {
		status = "partial"
	}
	items := make([]domain.SourceError, 0, len(errors))
	for _, value := range errors {
		items = append(items, domain.SourceError{
			Source: SourceCode, Code: value.Code, Message: value.Message,
			Retryable: value.Retryable,
		})
	}
	return domain.ObjectResolution{Ref: ref, Status: status, Errors: items}
}

func dedupeObjectRefs(values []domain.SourceObjectRef) []domain.SourceObjectRef {
	result := make([]domain.SourceObjectRef, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := value.SourceCode + "\x00" + value.SourceInstance + "\x00" + value.RecordType + "\x00" + value.ExternalID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func putSafeAttribute(target map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		target[key] = value
	}
}

func utcTimeRange(value domain.TimeRange) domain.TimeRange {
	return domain.TimeRange{From: value.From.UTC(), To: value.To.UTC()}
}

func objectURN(recordType, externalID string) string {
	return "urn:pt-maxpatrol-siem:" + recordType + ":" + externalID
}
