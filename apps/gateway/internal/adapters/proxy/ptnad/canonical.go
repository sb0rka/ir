package ptnad

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

func canonicalFinding(value Attack) domain.Finding {
	ref := canonicalRef(value.SourceRef)
	mentions := make([]domain.EntityMention, 0, 6)
	mentions = append(mentions, endpointMentions(value.Attacker, "attacker", value.SourceRef.SourceInstance)...)
	mentions = append(mentions, endpointMentions(value.Victim, "victim", value.SourceRef.SourceInstance)...)
	mentions = normalizeMentions(mentions)
	details := &domain.NADAttackDetails{
		Class: value.Class, GID: int(value.GID), SID: int(value.SID), Revision: int(value.Revision),
		FalsePositive: cloneBool(value.FalsePositive),
	}
	if value.RawPriority != nil {
		details.RawPriority = int(*value.RawPriority)
	}
	finding := domain.Finding{
		Ref: ref, Kind: AttackRecordType, Title: value.Title, Description: value.Description,
		Severity: value.Severity, OccurredAt: value.OccurredAt, Entities: mentions,
		NADAttack: details, SourceRef: objectURN(ref), FetchedAt: value.FetchedAt,
	}
	if value.ParentSession != nil {
		finding.RelatedSessions = []domain.SourceObjectRef{canonicalRef(*value.ParentSession)}
	}
	if value.SID != 0 || value.GID != 0 {
		finding.Rule = &domain.RuleRef{
			ID:   fmt.Sprintf("%d:%d:%d", value.GID, value.SID, value.Revision),
			Name: value.Title,
		}
	}
	return finding
}

func canonicalSession(value Session) domain.Session {
	source, destination := enrichedSessionEndpoints(value)
	ref := canonicalRef(value.SourceRef)
	end := value.End
	duration := value.DurationSeconds
	hasFiles := value.HasFiles
	mentions := make([]domain.EntityMention, 0, 6)
	mentions = append(mentions, endpointMentions(source, "src", value.SourceRef.SourceInstance)...)
	mentions = append(mentions, endpointMentions(destination, "dst", value.SourceRef.SourceInstance)...)
	item := domain.Session{
		Ref: ref, Title: sessionTitle(value, source, destination), Severity: value.Severity,
		StartedAt: value.Start, EndedAt: &end, DurationSeconds: &duration,
		SourceEndpoint: domain.NetworkEndpoint{
			IP: source.IP, MAC: source.MAC, Host: source.Host, Port: int(source.Port),
		},
		DestinationEndpoint: domain.NetworkEndpoint{
			IP: destination.IP, MAC: destination.MAC, Host: destination.Host, Port: int(destination.Port),
		},
		TransportProtocol: value.TransportProtocol, ApplicationProtocol: value.ApplicationProtocol,
		Bytes:   &domain.TrafficCounters{Sent: value.Bytes.Sent, Received: value.Bytes.Received, Total: value.Bytes.Total},
		Packets: &domain.TrafficCounters{Sent: value.Packets.Sent, Received: value.Packets.Received, Total: value.Packets.Total},
		State:   append([]string(nil), value.State...), FalsePositive: cloneBool(value.FalsePositive),
		HasFiles: &hasFiles, TCPFlags: append([]string(nil), value.TCPFlags...),
		FileHints: sessionFileHints(value.Files), AuthenticationHints: sessionAuthenticationHints(value),
		Entities: normalizeMentions(mentions), SourceRef: objectURN(ref), FetchedAt: value.FetchedAt,
	}
	if value.Criticality != nil {
		raw := int(*value.Criticality)
		item.RawCriticality = &raw
	}
	for _, attack := range value.RelatedAttacks {
		item.RelatedFindings = append(item.RelatedFindings, canonicalRef(attack.SourceRef))
	}
	item.RelatedFindings = normalizeObjectRefs(item.RelatedFindings)
	return item
}

func sessionFileHints(values []FileHint) []domain.SessionFileHint {
	result := make([]domain.SessionFileHint, 0, len(values))
	for _, value := range values {
		result = append(result, domain.SessionFileHint{
			ExternalID: value.ExternalID, Name: value.Name, MIME: value.MIME, Size: value.Size,
			MD5: value.MD5, SHA256: value.SHA256, State: value.State, Direction: value.Direction,
		})
	}
	return result
}

func sessionAuthenticationHints(value Session) []domain.SessionAuthenticationHint {
	result := make([]domain.SessionAuthenticationHint, 0, len(value.Authentication)+len(value.SSH))
	for _, hint := range value.Authentication {
		result = append(result, domain.SessionAuthenticationHint{
			Protocol: hint.Protocol, Account: hint.Account, Valid: cloneBool(hint.Valid),
			ClientHost: hint.ClientHost, ServerHost: hint.ServerHost,
		})
	}
	for _, hint := range value.SSH {
		result = append(result, domain.SessionAuthenticationHint{
			Protocol: "ssh", Method: hint.Authentication, FailedAttempts: cloneInt64(hint.FailedPasswordCount),
		})
	}
	return result
}

func sessionTitle(value Session, source, destination Endpoint) string {
	protocol := value.ApplicationProtocol
	if protocol == "" {
		protocol = value.TransportProtocol
	}
	if protocol == "" {
		protocol = "network"
	}
	return fmt.Sprintf("%s session %s:%d → %s:%d", strings.ToUpper(protocol), source.IP, source.Port, destination.IP, destination.Port)
}

func appendSessionContext(page *capability.ContextPage, value Session) {
	item := canonicalSession(value)
	page.Sessions = append(page.Sessions, item)
	resolution := domain.ObjectResolution{Ref: item.Ref, Status: "complete", Errors: []domain.SourceError{}}
	for _, err := range value.ContextErrors {
		resolution.Status = "partial"
		resolution.Errors = append(resolution.Errors, contextWarning("session HTTP transaction pagination", err))
	}
	page.Resolutions = append(page.Resolutions, resolution)
	for _, attack := range value.RelatedAttacks {
		finding := canonicalFinding(attack)
		page.Findings = append(page.Findings, finding)
		page.Resolutions = append(page.Resolutions, domain.ObjectResolution{Ref: finding.Ref, Status: "complete", Errors: []domain.SourceError{}})
	}
	events, entities, relations := decomposeSession(value)
	page.Events = append(page.Events, events...)
	page.Entities = append(page.Entities, entities...)
	page.Relations = append(page.Relations, relations...)
}

func appendAttackContext(page *capability.ContextPage, value Attack) {
	event := attackEvent(value)
	page.Events = append(page.Events, event)
	page.Entities = append(page.Entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	for _, endpoint := range []Endpoint{value.Attacker, value.Victim} {
		page.Relations = append(page.Relations, endpointIdentifierRelations(endpoint, value.SourceRef.SourceInstance, value.OccurredAt, event.Provenance)...)
	}
}

func decomposeSession(value Session) ([]domain.Event, []domain.Entity, []domain.Relation) {
	sessionEvent := sessionEvent(value)
	events := []domain.Event{sessionEvent}
	entities := entitiesForEvent(sessionEvent, value.SourceRef.SourceInstance)
	relations := sessionRelations(value)

	for _, attack := range value.RelatedAttacks {
		event := attackEvent(attack)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, attack.SourceRef.SourceInstance)...)
		for _, endpoint := range []Endpoint{attack.Attacker, attack.Victim} {
			relations = append(relations, endpointIdentifierRelations(endpoint, attack.SourceRef.SourceInstance, attack.OccurredAt, event.Provenance)...)
		}
	}
	for _, file := range value.Files {
		event := fileEvent(value, file)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	for index, authentication := range value.Authentication {
		event := authenticationEvent(value, authentication, index)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	for _, hint := range value.SSH {
		event := sshEvent(value, hint)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	for _, hint := range value.HTTP {
		event := httpEvent(value, hint)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	for _, hint := range value.SMB {
		event := smbEvent(value, hint)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	for _, hint := range value.DCERPC {
		event := dcerpcEvent(value, hint)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	for _, hint := range value.NTLM {
		event := ntlmEvent(value, hint)
		events = append(events, event)
		entities = append(entities, entitiesForEvent(event, value.SourceRef.SourceInstance)...)
	}
	return normalization.Events(events), normalization.Entities(entities), normalization.Relations(relations)
}

func sessionEvent(value Session) domain.Event {
	item := canonicalSession(value)
	attributes := map[string]any{
		"ended_at": item.EndedAt, "duration_seconds": item.DurationSeconds,
		"source_port": item.SourceEndpoint.Port, "destination_port": item.DestinationEndpoint.Port,
		"transport_protocol": item.TransportProtocol, "application_protocol": item.ApplicationProtocol,
		"state": append([]string(nil), item.State...), "tcp_flags": append([]string(nil), item.TCPFlags...),
		"has_files": value.HasFiles,
	}
	if len(value.ApplicationProtocols) > 0 {
		attributes["application_protocols"] = append([]string(nil), value.ApplicationProtocols...)
	}
	if len(value.Errors) > 0 {
		attributes["errors"] = append([]string(nil), value.Errors...)
	}
	if len(value.TCPFlagsClient) > 0 {
		attributes["tcp_flags_client"] = append([]string(nil), value.TCPFlagsClient...)
	}
	if len(value.TCPFlagsServer) > 0 {
		attributes["tcp_flags_server"] = append([]string(nil), value.TCPFlagsServer...)
	}
	if value.StoreTag != "" {
		attributes["store_tag"] = value.StoreTag
	}
	if value.StorageIndex != "" {
		attributes["storage_index"] = value.StorageIndex
	}
	if len(value.Banners.Client) > 0 || len(value.Banners.Server) > 0 {
		attributes["banners"] = map[string][]string{
			"client": append([]string(nil), value.Banners.Client...),
			"server": append([]string(nil), value.Banners.Server...),
		}
	}
	if len(value.OperatingSystems.Client) > 0 || len(value.OperatingSystems.Server) > 0 {
		attributes["operating_systems"] = map[string][]string{
			"client": append([]string(nil), value.OperatingSystems.Client...),
			"server": append([]string(nil), value.OperatingSystems.Server...),
		}
	}
	if item.Bytes != nil {
		attributes["bytes"] = map[string]int64{"sent": item.Bytes.Sent, "received": item.Bytes.Received, "total": item.Bytes.Total}
	}
	if item.Packets != nil {
		attributes["packets"] = map[string]int64{"sent": item.Packets.Sent, "received": item.Packets.Received, "total": item.Packets.Total}
	}
	if value.Criticality != nil {
		attributes["criticality"] = *value.Criticality
	}
	return domain.Event{
		Type: "network.session", Title: item.Title, Severity: item.Severity, OccurredAt: item.StartedAt,
		Entities: item.Entities, Attributes: attributes, Provenance: objectProvenance(value.SourceRef, value.FetchedAt),
	}
}

func attackEvent(value Attack) domain.Event {
	finding := canonicalFinding(value)
	attributes := map[string]any{
		"class": value.Class, "gid": value.GID, "sid": value.SID, "revision": value.Revision,
	}
	if value.RawPriority != nil {
		attributes["raw_priority"] = *value.RawPriority
	}
	if value.FalsePositive != nil {
		attributes["false_positive"] = *value.FalsePositive
	}
	if value.ParentSession != nil {
		attributes["parent_session_id"] = value.ParentSession.ExternalID
	}
	if value.Recommendation != "" {
		attributes["recommendation"] = value.Recommendation
	}
	if value.RuleVendor != "" {
		attributes["rule_vendor"] = value.RuleVendor
	}
	if value.AttackTarget != "" {
		attributes["attack_target"] = value.AttackTarget
	}
	if value.AttackFlag != nil {
		attributes["attack_flag"] = *value.AttackFlag
	}
	if value.RuleDisabled != nil {
		attributes["rule_disabled"] = *value.RuleDisabled
	}
	if value.MatchType != "" {
		attributes["match_type"] = value.MatchType
	}
	if len(value.ATTACK) > 0 {
		attributes["attack_techniques"] = append([]string(nil), value.ATTACK...)
	}
	if value.Direction != "" {
		attributes["direction"] = value.Direction
	}
	return domain.Event{
		Type: "network.detection", Title: finding.Title, Severity: "unknown", OccurredAt: finding.OccurredAt,
		Entities: finding.Entities, Attributes: attributes, Provenance: objectProvenance(value.SourceRef, value.FetchedAt),
	}
}

func fileEvent(session Session, file FileHint) domain.Event {
	mentions := append([]domain.EntityMention(nil), sessionEndpointMentions(session)...)
	if file.SHA256 != "" {
		mentions = append(mentions, mention("file_hash", file.SHA256, "object"))
	} else if file.MD5 != "" {
		mentions = append(mentions, mention("file_hash", file.MD5, "object"))
	}
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID, "vendor_file_id": file.VendorID,
		"name": file.Name, "path": file.Path, "mime": file.MIME, "magic": file.Magic,
		"size": file.Size, "state": file.State, "direction": file.Direction,
	}
	if file.MD5 != "" {
		attributes["md5"] = file.MD5
	}
	if file.SHA256 != "" {
		attributes["sha256"] = file.SHA256
	}
	return domain.Event{
		Type: "network.file", Title: firstNonEmpty(file.Name, "Network file"), Severity: "info",
		OccurredAt: session.Start, Entities: normalizeMentions(mentions), Attributes: attributes,
		Provenance: childProvenance(session, "nad_file", file.ExternalID),
	}
}

func authenticationEvent(session Session, hint AuthenticationHint, index int) domain.Event {
	mentions := append([]domain.EntityMention(nil), sessionEndpointMentions(session)...)
	if hint.Account != "" {
		mentions = append(mentions, mention("account", hint.Account, "actor"))
	}
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID, "protocol": hint.Protocol,
		"client_host": hint.ClientHost, "server_host": hint.ServerHost,
	}
	if hint.Valid != nil {
		attributes["valid"] = *hint.Valid
	}
	return domain.Event{
		Type: "network.authentication", Title: strings.ToUpper(firstNonEmpty(hint.Protocol, "network")) + " authentication",
		Severity: "info", OccurredAt: session.Start, Entities: normalizeMentions(mentions), Attributes: attributes,
		Provenance: childProvenance(session, "nad_auth", strconv.Itoa(index)),
	}
}

func sshEvent(session Session, hint SSHHint) domain.Event {
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID,
		"transaction_id":    hint.Transaction.TxID,
		"protocol":          "ssh",
		"authentication":    hint.Authentication,
		"compression":       append([]string(nil), hint.Compression...),
		"encryption":        append([]string(nil), hint.Encryption...),
		"client_protocol":   hint.ClientProtocol,
		"client_software":   hint.ClientSoftware,
		"server_protocol":   hint.ServerProtocol,
		"server_software":   hint.ServerSoftware,
	}
	if hint.FailedPasswordCount != nil {
		attributes["failed_password_count"] = *hint.FailedPasswordCount
	}
	if hint.KeyPressed != nil {
		attributes["key_pressed"] = *hint.KeyPressed
	}
	title := "SSH authentication"
	if hint.Authentication != "" {
		title = "SSH authentication: " + hint.Authentication
	}
	return protocolEvent(session, hint.Transaction, "network.authentication", title, sessionEndpointMentions(session), attributes)
}

func httpEvent(session Session, hint HTTPHint) domain.Event {
	mentions := append([]domain.EntityMention(nil), sessionEndpointMentions(session)...)
	if hint.Host != "" {
		mentions = append(mentions, mention("domain", hint.Host, "object"))
	}
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID, "transaction_id": hint.Transaction.TxID,
		"method": hint.Method, "path": hint.Path, "host": hint.Host,
		"request_bytes": hint.RequestBytes, "request_content_type": hint.RequestContentType,
		"response_code": hint.ResponseCode, "response_status": hint.ResponseStatus,
		"response_bytes": hint.ResponseBytes, "response_server": hint.ResponseServer,
		"response_content_type": hint.ResponseContentType,
	}
	return protocolEvent(session, hint.Transaction, "network.http", firstNonEmpty(hint.Method+" "+hint.Path, "HTTP transaction"), mentions, attributes)
}

func smbEvent(session Session, hint SMBHint) domain.Event {
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID, "transaction_id": hint.Transaction.TxID,
		"command": hint.Command, "status": hint.Status, "filename": hint.Filename,
		"action": hint.Action, "tree_path": hint.TreePath, "share_type": hint.ShareType,
	}
	return protocolEvent(session, hint.Transaction, "network.smb", firstNonEmpty(hint.Command, "SMB transaction"), sessionEndpointMentions(session), attributes)
}

func dcerpcEvent(session Session, hint DCERPCHint) domain.Event {
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID, "transaction_id": hint.Transaction.TxID,
		"packet_type": hint.PacketType, "interface": hint.Interface, "operation": hint.Operation,
		"auth_type": hint.AuthType, "auth_level": hint.AuthLevel, "arguments_decoded": false,
	}
	return protocolEvent(session, hint.Transaction, "network.dcerpc", firstNonEmpty(hint.Operation, hint.PacketType, "DCERPC transaction"), sessionEndpointMentions(session), attributes)
}

func ntlmEvent(session Session, hint NTLMHint) domain.Event {
	mentions := append([]domain.EntityMention(nil), sessionEndpointMentions(session)...)
	if hint.Account != "" {
		mentions = append(mentions, mention("account", hint.Account, "actor"))
	}
	if hint.ClientHost != "" {
		mentions = append(mentions, mention("host", hint.ClientHost, "src"))
	}
	if hint.TargetHost != "" {
		mentions = append(mentions, mention("host", hint.TargetHost, "dst"))
	}
	attributes := map[string]any{
		"parent_session_id": session.SourceRef.ExternalID, "transaction_id": hint.Transaction.TxID,
		"message_type": hint.MessageType, "client_host": hint.ClientHost,
		"target_host": hint.TargetHost, "domain": hint.Domain,
		"os_version": hint.OSVersion, "os_build": hint.OSBuild,
	}
	return protocolEvent(session, hint.Transaction, "network.authentication", firstNonEmpty(hint.MessageType, "NTLM authentication"), mentions, attributes)
}

func protocolEvent(session Session, transaction TransactionRef, eventType, title string, mentions []domain.EntityMention, attributes map[string]any) domain.Event {
	return domain.Event{
		Type: eventType, Title: strings.TrimSpace(title), Severity: "info", OccurredAt: transaction.OccurredAt,
		Entities: normalizeMentions(mentions), Attributes: attributes,
		Provenance: childProvenance(session, strings.TrimPrefix(eventType, "network."), transaction.ExternalID),
	}
}

func sessionRelations(value Session) []domain.Relation {
	provenance := objectProvenance(value.SourceRef, value.FetchedAt)
	result := make([]domain.Relation, 0)
	source, destination := enrichedSessionEndpoints(value)
	if source.IP != "" && destination.IP != "" {
		result = append(result, relation("connected_to", entityRef("ip", source.IP), entityRef("ip", destination.IP), value.Start, provenance))
	}
	for _, endpoint := range []Endpoint{source, destination} {
		result = append(result, endpointIdentifierRelations(endpoint, value.SourceRef.SourceInstance, value.Start, provenance)...)
		if endpoint.Host != "" && endpoint.IP != "" {
			result = append(result, relation("has_interface", entityRef("host", endpoint.Host), entityRef("ip", endpoint.IP), value.Start, provenance))
		}
		if endpoint.Host != "" && endpoint.MAC != "" {
			result = append(result, relation("has_interface", entityRef("host", endpoint.Host), entityRef("mac", endpoint.MAC), value.Start, provenance))
		}
	}
	for _, hint := range value.Authentication {
		if hint.Account == "" {
			continue
		}
		target := entityRef("host", firstNonEmpty(hint.ServerHost, destination.Host))
		if target.Value == "" {
			target = entityRef("ip", destination.IP)
		}
		if target.Value != "" {
			result = append(result, relation("authenticated_to", entityRef("account", hint.Account), target, value.Start, provenance))
		}
	}
	for _, file := range value.Files {
		hash := firstNonEmpty(file.SHA256, file.MD5)
		if hash == "" {
			continue
		}
		targetEndpoint := destination
		if file.Direction == "destination_to_source" {
			targetEndpoint = source
		}
		target := entityRef("host", targetEndpoint.Host)
		if target.Value == "" {
			target = entityRef("ip", targetEndpoint.IP)
		}
		if target.Value != "" {
			result = append(result, relation("transferred_to", entityRef("file_hash", hash), target, value.Start, provenance))
		}
	}
	return result
}

func enrichedSessionEndpoints(value Session) (Endpoint, Endpoint) {
	source := value.Source
	destination := value.Destination
	for _, hint := range value.NTLM {
		if source.Host == "" && hint.ClientHost != "" {
			source.Host = hint.ClientHost
		}
		if destination.Host == "" && hint.TargetHost != "" {
			destination.Host = hint.TargetHost
		}
	}
	return source, destination
}

func relation(kind string, source, target domain.EntityRef, occurredAt time.Time, provenance domain.Provenance) domain.Relation {
	provenance.ExternalID = provenance.ExternalID + ":relation:" + kind + ":" + source.Type + ":" + source.Value + ":" + target.Type + ":" + target.Value
	return domain.Relation{Type: kind, SourceEntity: source, TargetEntity: target, OccurredAt: &occurredAt, Provenance: provenance}
}

func entitiesForEvent(event domain.Event, sourceInstance string) []domain.Entity {
	result := make([]domain.Entity, 0, len(event.Entities))
	for _, mention := range event.Entities {
		provenance := event.Provenance
		provenance.ExternalID = entitySourceID(sourceInstance, mention.Type, mention.Value)
		entity := domain.NewEntity(mention.Type, mention.Value, provenance)
		if mention.Type == "device" {
			entity.Attributes["identity_method"] = "pt-nad-host-id"
			entity.Attributes["source_instance"] = sourceInstance
		}
		result = append(result, entity)
	}
	return result
}

func sessionEndpointMentions(value Session) []domain.EntityMention {
	mentions := make([]domain.EntityMention, 0, 6)
	mentions = append(mentions, endpointMentions(value.Source, "src", value.SourceRef.SourceInstance)...)
	mentions = append(mentions, endpointMentions(value.Destination, "dst", value.SourceRef.SourceInstance)...)
	return normalizeMentions(mentions)
}

func endpointMentions(value Endpoint, role, sourceInstance string) []domain.EntityMention {
	result := make([]domain.EntityMention, 0, 3)
	if device, ok := endpointDevice(value, sourceInstance); ok {
		result = append(result, domain.EntityMention{EntityRef: device, Roles: []string{role}})
	}
	if value.IP != "" {
		result = append(result, mention("ip", value.IP, role))
	}
	if value.MAC != "" {
		result = append(result, mention("mac", value.MAC, role))
	}
	if host := firstNonEmpty(value.Host, value.DNS); host != "" {
		result = append(result, mention("host", host, role))
	}
	return result
}

func endpointDevice(value Endpoint, sourceInstance string) (domain.EntityRef, bool) {
	if strings.TrimSpace(value.HostID) == "" || strings.TrimSpace(sourceInstance) == "" {
		return domain.EntityRef{}, false
	}
	// HostID is meaningful only in its source instance; weak identifiers never substitute for it.
	id := domain.StableID("pt-nad", sourceInstance, "host-id", strings.TrimSpace(value.HostID))
	return entityRef("device", "pt-nad:"+id.String()), true
}

func endpointIdentifierRelations(endpoint Endpoint, sourceInstance string, at time.Time, provenance domain.Provenance) []domain.Relation {
	device, ok := endpointDevice(endpoint, sourceInstance)
	if !ok {
		return nil
	}
	var result []domain.Relation
	for _, identifier := range []domain.EntityRef{entityRef("ip", endpoint.IP), entityRef("mac", endpoint.MAC), entityRef("host", firstNonEmpty(endpoint.Host, endpoint.DNS))} {
		if identifier.Value != "" {
			result = append(result, relation("has_identifier", device, identifier, at, provenance))
		}
	}
	return result
}

func mention(kind, value, role string) domain.EntityMention {
	return domain.EntityMention{EntityRef: entityRef(kind, value), Roles: []string{role}}
}

func entityRef(kind, value string) domain.EntityRef {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return domain.EntityRef{Type: kind, Value: domain.CanonicalValue(kind, value)}
}

func normalizeMentions(values []domain.EntityMention) []domain.EntityMention {
	seen := make(map[string]domain.EntityMention, len(values))
	for _, value := range values {
		value.EntityRef = entityRef(value.Type, value.Value)
		if value.Value == "" {
			continue
		}
		key := value.Type + "\x00" + value.Value
		current := seen[key]
		current.EntityRef = value.EntityRef
		roleSet := make(map[string]struct{}, len(current.Roles)+len(value.Roles))
		for _, role := range append(current.Roles, value.Roles...) {
			if role = strings.ToLower(strings.TrimSpace(role)); role != "" {
				roleSet[role] = struct{}{}
			}
		}
		current.Roles = current.Roles[:0]
		for role := range roleSet {
			current.Roles = append(current.Roles, role)
		}
		sort.Strings(current.Roles)
		seen[key] = current
	}
	result := make([]domain.EntityMention, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Type != result[right].Type {
			return result[left].Type < result[right].Type
		}
		return result[left].Value < result[right].Value
	})
	return result
}

func normalizeObjectRefs(values []domain.SourceObjectRef) []domain.SourceObjectRef {
	seen := make(map[string]domain.SourceObjectRef, len(values))
	for _, value := range values {
		seen[objectIdentity(value)] = value
	}
	result := make([]domain.SourceObjectRef, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return objectIdentity(result[left]) < objectIdentity(result[right]) })
	return result
}

func normalizeContextPage(page capability.ContextPage) capability.ContextPage {
	page.Events = normalization.Events(page.Events)
	page.Entities = normalization.Entities(page.Entities)
	page.Relations = normalization.Relations(page.Relations)
	findings := make(map[string]domain.Finding, len(page.Findings))
	for _, value := range page.Findings {
		findings[objectIdentity(value.Ref)] = value
	}
	page.Findings = page.Findings[:0]
	for _, value := range findings {
		page.Findings = append(page.Findings, value)
	}
	sort.Slice(page.Findings, func(left, right int) bool {
		return objectIdentity(page.Findings[left].Ref) < objectIdentity(page.Findings[right].Ref)
	})
	sessions := make(map[string]domain.Session, len(page.Sessions))
	for _, value := range page.Sessions {
		sessions[objectIdentity(value.Ref)] = value
	}
	page.Sessions = page.Sessions[:0]
	for _, value := range sessions {
		page.Sessions = append(page.Sessions, value)
	}
	sort.Slice(page.Sessions, func(left, right int) bool {
		return objectIdentity(page.Sessions[left].Ref) < objectIdentity(page.Sessions[right].Ref)
	})
	resolutions := make(map[string]domain.ObjectResolution, len(page.Resolutions))
	for _, value := range page.Resolutions {
		key := objectIdentity(value.Ref)
		current, exists := resolutions[key]
		if !exists || current.Status != "partial" {
			resolutions[key] = value
		}
	}
	page.Resolutions = page.Resolutions[:0]
	for _, value := range resolutions {
		if value.Errors == nil {
			value.Errors = []domain.SourceError{}
		}
		page.Resolutions = append(page.Resolutions, value)
	}
	sort.Slice(page.Resolutions, func(left, right int) bool {
		return objectIdentity(page.Resolutions[left].Ref) < objectIdentity(page.Resolutions[right].Ref)
	})
	return page
}

func canonicalRef(value SourceRef) domain.SourceObjectRef {
	return domain.SourceObjectRef{
		SourceCode: value.SourceCode, SourceInstance: value.SourceInstance,
		RecordType: value.RecordType, ExternalID: value.ExternalID,
		TimeRange: domain.TimeRange{From: value.TimeRange.From, To: value.TimeRange.To},
	}
}

func objectIdentity(ref domain.SourceObjectRef) string {
	return ref.SourceCode + "\x00" + ref.SourceInstance + "\x00" + ref.RecordType + "\x00" + ref.ExternalID
}

func objectURN(ref domain.SourceObjectRef) string {
	return "urn:pt-nad:" + ref.SourceInstance + ":" + ref.RecordType + ":" + ref.ExternalID
}

func objectProvenance(ref SourceRef, fetchedAt time.Time) domain.Provenance {
	canonical := canonicalRef(ref)
	return domain.Provenance{
		Source: SourceCode, ExternalID: sourceEventID(ref), SourceURL: objectURN(canonical), FetchedAt: fetchedAt,
	}
}

func childProvenance(session Session, recordType, externalID string) domain.Provenance {
	return domain.Provenance{
		Source:     SourceCode,
		ExternalID: session.SourceRef.SourceInstance + ":" + recordType + ":" + session.SourceRef.ExternalID + ":" + externalID,
		SourceURL:  objectURN(canonicalRef(session.SourceRef)), FetchedAt: session.FetchedAt,
	}
}

func sourceEventID(ref SourceRef) string {
	return ref.SourceInstance + ":" + ref.RecordType + ":" + ref.ExternalID
}
