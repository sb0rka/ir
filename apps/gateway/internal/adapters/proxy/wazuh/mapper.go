package wazuh

import (
	"net"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type mappedAlert struct {
	Event     domain.Event
	Entities  []domain.Entity
	Relations []domain.Relation
}

type mentionState struct {
	ref   domain.EntityRef
	roles []string
}

type entityBuilder struct {
	fetchedAt    time.Time
	eventID      string
	occurredAt   time.Time
	entities     []domain.Entity
	relations    []domain.Relation
	mentionRoles []mentionState
	mentionIdx   map[string]int
	entityIdx    map[string]struct{}
	relationIdx  map[string]struct{}
}

func mapHit(index, documentID string, source alertSource, fetchedAt time.Time) (mappedAlert, error) {
	index = strings.TrimSpace(index)
	documentID = strings.TrimSpace(documentID)
	if index == "" || documentID == "" {
		return mappedAlert{}, &ResponseError{Operation: "event search", Message: "alert identity is missing"}
	}
	occurredAt, err := parseOccurredAt(source)
	if err != nil {
		return mappedAlert{}, err
	}
	externalID := index + "/" + documentID
	title := boundText(source.Rule.Description, 512)
	if title == "" {
		title = "Wazuh rule " + boundText(source.Rule.ID, 64)
	}
	attrs := map[string]any{}
	putAttr(attrs, "rule.id", source.Rule.ID)
	putAttrInt(attrs, "rule.level", source.Rule.Level)
	putAttrList(attrs, "rule.groups", source.Rule.Groups)
	putAttrInt(attrs, "rule.firedtimes", source.Rule.FiredTimes)
	putAttrInt(attrs, "rule.frequency", source.Rule.Frequency)
	putAttrList(attrs, "rule.mitre.id", source.Rule.Mitre.ID)
	putAttrList(attrs, "rule.mitre.tactic", source.Rule.Mitre.Tactic)
	putAttrList(attrs, "rule.mitre.technique", source.Rule.Mitre.Technique)
	putAttr(attrs, "agent.id", source.Agent.ID)
	putAttr(attrs, "agent.name", source.Agent.Name)
	putAttr(attrs, "agent.ip", source.Agent.IP)
	putAttr(attrs, "manager.name", source.Manager.Name)
	putAttr(attrs, "decoder.name", source.Decoder.Name)
	putAttr(attrs, "location", source.Location)
	putAttr(attrs, "GeoLocation.country_name", source.GeoLocation.CountryName)
	putAttr(attrs, "GeoLocation.city_name", source.GeoLocation.CityName)

	if source.Rule.Frequency > 0 {
		putAttr(attrs, "correlation_name", source.Rule.Description)
		putAttr(attrs, "correlation_type", "wazuh_frequency")
	} else {
		putAttr(attrs, "correlation_type", "wazuh_rule")
	}

	builder := &entityBuilder{
		fetchedAt:   fetchedAt,
		eventID:     externalID,
		occurredAt:  occurredAt,
		mentionIdx:  map[string]int{},
		entityIdx:   map[string]struct{}{},
		relationIdx: map[string]struct{}{},
	}
	addFamilyContext(builder, attrs, source)

	event := domain.Event{
		Type:       mapEventType(source.Rule.Groups),
		Title:      title,
		Severity:   mapSeverity(source.Rule.Level),
		OccurredAt: occurredAt,
		Entities:   builder.mentions(),
		Attributes: attrs,
		Provenance: domain.Provenance{
			Source:     SourceCode,
			ExternalID: externalID,
			SourceURL:  "wazuh://" + externalID,
			FetchedAt:  fetchedAt,
		},
	}
	return mappedAlert{Event: event, Entities: builder.entities, Relations: builder.relations}, nil
}

func (builder *entityBuilder) add(kind, value string, roles ...string) domain.EntityRef {
	kind = strings.ToLower(strings.TrimSpace(kind))
	value = domain.CanonicalValue(kind, value)
	if kind == "" || value == "" {
		return domain.EntityRef{}
	}
	if kind == "ip" && net.ParseIP(value) == nil {
		return domain.EntityRef{}
	}
	switch kind {
	case "file_hash", "md5", "sha1", "sha256", "hash":
		kind = "file_hash"
		if !validHash(value) {
			return domain.EntityRef{}
		}
	}
	ref := domain.EntityRef{Type: kind, Value: value}
	key := kind + "\x00" + value
	if index, ok := builder.mentionIdx[key]; ok {
		builder.mentionRoles[index].roles = mergeRoles(builder.mentionRoles[index].roles, roles)
	} else {
		builder.mentionIdx[key] = len(builder.mentionRoles)
		builder.mentionRoles = append(builder.mentionRoles, mentionState{ref: ref, roles: mergeRoles(nil, roles)})
	}
	if _, exists := builder.entityIdx[key]; !exists {
		builder.entityIdx[key] = struct{}{}
		provenance := domain.Provenance{
			Source:     SourceCode,
			ExternalID: kind + ":" + value,
			FetchedAt:  builder.fetchedAt,
		}
		builder.entities = append(builder.entities, domain.NewEntity(kind, value, provenance))
	}
	return ref
}

func (builder *entityBuilder) relate(relationType string, source, target domain.EntityRef) {
	if source.Type == "" || target.Type == "" || relationType == "" {
		return
	}
	id := builder.eventID + ":" + relationType + ":" + source.Type + ":" + source.Value + ":" + target.Type + ":" + target.Value
	if _, exists := builder.relationIdx[id]; exists {
		return
	}
	builder.relationIdx[id] = struct{}{}
	occurred := builder.occurredAt
	builder.relations = append(builder.relations, domain.Relation{
		Type:         relationType,
		SourceEntity: source,
		TargetEntity: target,
		OccurredAt:   &occurred,
		Provenance: domain.Provenance{
			Source:     SourceCode,
			ExternalID: id,
			FetchedAt:  builder.fetchedAt,
		},
	})
}

func (builder *entityBuilder) mentions() []domain.EntityMention {
	result := make([]domain.EntityMention, 0, len(builder.mentionRoles))
	for _, item := range builder.mentionRoles {
		result = append(result, domain.EntityMention{EntityRef: item.ref, Roles: append([]string(nil), item.roles...)})
	}
	return result
}

func mergeRoles(existing, added []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(existing)+len(added))
	for _, role := range append(existing, added...) {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		result = append(result, role)
	}
	return result
}

func validHash(value string) bool {
	switch len(value) {
	case 32, 40, 64:
	default:
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func addFamilyContext(builder *entityBuilder, attrs map[string]any, source alertSource) {
	agentHost := builder.add("host", source.Agent.Name, "mentions")
	agentIP := builder.add("ip", source.Agent.IP, "mentions")
	if agentHost.Type != "" && agentIP.Type != "" {
		builder.relate("has_interface", agentHost, agentIP)
	}

	groups := source.Rule.Groups
	eventType := mapEventType(groups)

	srcIP := builder.add("ip", source.Data.SrcIP, "src")
	dstIP := builder.add("ip", source.Data.DstIP, "dst")
	srcUser := builder.add("account", source.Data.SrcUser, "actor")
	_ = builder.add("account", source.Data.DstUser, "object", "account")
	putAttr(attrs, "data.srcport", source.Data.SrcPort)
	putAttr(attrs, "data.dstport", source.Data.DstPort)
	putAttr(attrs, "data.protocol", source.Data.Protocol)
	putAttr(attrs, "data.id", source.Data.ID)
	putAttr(attrs, "data.url", source.Data.URL)
	putAttr(attrs, "data.status", source.Data.Status)
	putAttr(attrs, "data.integration", source.Data.Integration)

	if source.Data.URL != "" {
		builder.add("url", source.Data.URL, "object")
	}

	if hasGroup(groups, "win_authentication_failed") || hasGroup(groups, "windows") {
		builder.add("account", source.Data.Win.EventData.TargetUserName, "account")
		builder.add("ip", source.Data.Win.EventData.IPAddress, "src")
		winHost := builder.add("host", source.Data.Win.System.Computer, "object")
		winIP := builder.add("ip", source.Data.Win.EventData.IPAddress)
		if winHost.Type != "" && winIP.Type != "" {
			builder.relate("connected_to", winIP, winHost)
		}
		builder.add("host", source.Data.SystemName, "object")
		builder.add("account", source.Data.DstUser, "account")
		putAttr(attrs, "data.win.system.eventID", source.Data.Win.System.EventID)
		putAttr(attrs, "data.win.system.channel", source.Data.Win.System.Channel)
		putAttr(attrs, "data.win.eventdata.logonType", source.Data.Win.EventData.LogonType)
	}

	if hasGroup(groups, "syscheck") {
		hash := source.Syscheck.SHA256After
		if hash == "" {
			hash = source.Syscheck.SHA1After
		}
		if hash == "" {
			hash = source.Syscheck.MD5After
		}
		builder.add("file_hash", hash, "file")
		putAttr(attrs, "syscheck.path", source.Syscheck.Path)
		putAttr(attrs, "syscheck.event", source.Syscheck.Event)
		putAttr(attrs, "syscheck.md5_after", source.Syscheck.MD5After)
		putAttr(attrs, "syscheck.sha1_after", source.Syscheck.SHA1After)
		putAttr(attrs, "syscheck.sha256_after", source.Syscheck.SHA256After)
	}

	if source.Data.AWS.Source != "" || hasGroup(groups, "aws") || hasGroup(groups, "aws_guardduty") {
		remote := source.Data.AWS.Service.Action.NetworkConnectionAction.RemoteIPDetails.IPAddressV4
		if remote == "" {
			remote = source.Data.AWS.Service.Action.AWSAPICallAction.RemoteIPDetails.IPAddressV4
		}
		local := source.Data.AWS.Service.Action.NetworkConnectionAction.LocalIPDetails.IPAddressV4
		direction := strings.ToUpper(source.Data.AWS.Service.Action.NetworkConnectionAction.ConnectionDirection)
		remoteRef := domain.EntityRef{}
		localRef := domain.EntityRef{}
		if direction == "INBOUND" {
			remoteRef = builder.add("ip", remote, "src")
			localRef = builder.add("ip", local, "object")
		} else {
			remoteRef = builder.add("ip", remote, "dst")
			localRef = builder.add("ip", local, "object")
		}
		if localRef.Type != "" && remoteRef.Type != "" {
			builder.relate("connected_to", localRef, remoteRef)
		}
		putAttr(attrs, "data.aws.accountId", source.Data.AWS.AccountID)
		putAttr(attrs, "data.aws.region", source.Data.AWS.Region)
		putAttr(attrs, "data.aws.type", source.Data.AWS.Type)
		putAttr(attrs, "data.aws.source", source.Data.AWS.Source)
		putAttr(attrs, "data.aws.title", source.Data.AWS.Title)
	}

	if hasGroup(groups, "office365") || source.Data.Office365.UserID != "" {
		builder.add("account", source.Data.Office365.UserID, "actor")
		builder.add("ip", source.Data.Office365.ClientIP, "src")
		putAttr(attrs, "data.office365.Operation", source.Data.Office365.Operation)
		putAttr(attrs, "data.office365.Workload", source.Data.Office365.Workload)
	}

	if hasGroup(groups, "ms-graph") || source.Data.MSGraph.IPAddress != "" {
		builder.add("ip", source.Data.MSGraph.IPAddress, "src")
		putAttr(attrs, "data.ms-graph.title", source.Data.MSGraph.Title)
		putAttr(attrs, "data.ms-graph.severity", source.Data.MSGraph.Severity)
		putAttr(attrs, "data.ms-graph.category", source.Data.MSGraph.Category)
	}

	if hasGroup(groups, "github") || source.Data.GitHub.Actor != "" {
		builder.add("account", source.Data.GitHub.Actor, "actor")
		builder.add("account", source.Data.GitHub.User, "object")
		putAttr(attrs, "data.github.org", source.Data.GitHub.Org)
		putAttr(attrs, "data.github.action", source.Data.GitHub.Action)
		putAttr(attrs, "data.github.repo", source.Data.GitHub.Repo)
	}

	if hasGroup(groups, "gcp") || source.Data.GCP.JSONPayload.SourceIP != "" {
		builder.add("ip", source.Data.GCP.JSONPayload.SourceIP, "src")
		builder.add("host", source.Data.GCP.JSONPayload.VMInstanceName, "object")
		builder.add("domain", strings.TrimSuffix(source.Data.GCP.JSONPayload.QueryName, "."), "object")
		putAttr(attrs, "data.gcp.resource.labels.project_id", source.Data.GCP.Resource.Labels.ProjectID)
	}

	if hasGroup(groups, "virustotal") || source.Data.VirusTotal.Source.SHA1 != "" {
		builder.add("file_hash", source.Data.VirusTotal.Source.SHA1, "file")
		builder.add("file_hash", source.Data.VirusTotal.Source.MD5, "file")
		putAttr(attrs, "data.virustotal.positives", source.Data.VirusTotal.Positives)
		putAttr(attrs, "data.virustotal.source.file", basename(source.Data.VirusTotal.Source.File))
		putAttr(attrs, "data.virustotal.source.md5", source.Data.VirusTotal.Source.MD5)
	}

	if hasGroup(groups, "vulnerability-detector") || source.Data.Vulnerability.CVE != "" {
		putAttr(attrs, "data.vulnerability.cve", source.Data.Vulnerability.CVE)
		putAttr(attrs, "data.vulnerability.package.name", source.Data.Vulnerability.Package.Name)
		putAttr(attrs, "data.vulnerability.package.version", source.Data.Vulnerability.Package.Version)
		putAttr(attrs, "data.vulnerability.severity", source.Data.Vulnerability.Severity)
		putAttr(attrs, "data.vulnerability.status", source.Data.Vulnerability.Status)
		if source.Data.Vulnerability.CVSS.CVSS3.BaseScore > 0 {
			attrs["data.vulnerability.cvss.cvss3.base_score"] = source.Data.Vulnerability.CVSS.CVSS3.BaseScore
		}
	}

	if hasGroup(groups, "audit_command") || hasGroup(groups, "sudo") || source.Data.Audit.Command != "" {
		putAttr(attrs, "data.audit.command", basename(source.Data.Audit.Command))
		if source.Data.Command != "" {
			putAttr(attrs, "data.audit.command", basename(source.Data.Command))
		}
		putAttr(attrs, "data.audit.exe", basename(source.Data.Audit.Exe))
		putAttr(attrs, "data.audit.success", source.Data.Audit.Success)
		putAttr(attrs, "data.audit.type", source.Data.Audit.Type)
	}

	if hasGroup(groups, "docker") {
		putAttr(attrs, "data.docker.Action", source.Data.Docker.Action)
		putAttr(attrs, "data.docker.Type", source.Data.Docker.Type)
	}
	if hasGroup(groups, "osquery") {
		putAttr(attrs, "data.osquery.name", source.Data.Osquery.Name)
		putAttr(attrs, "data.osquery.action", source.Data.Osquery.Action)
		putAttr(attrs, "data.osquery.pack", source.Data.Osquery.Pack)
	}

	if eventType == "authentication.success" || eventType == "authentication.failure" {
		if srcUser.Type != "" && agentHost.Type != "" {
			builder.relate("authenticated_to", srcUser, agentHost)
		}
		if srcIP.Type != "" && agentHost.Type != "" {
			builder.relate("connected_to", srcIP, agentHost)
		}
	} else if srcIP.Type != "" && agentHost.Type != "" && (hasGroup(groups, "web") || hasGroup(groups, "accesslog") || hasGroup(groups, "sshd")) {
		builder.relate("connected_to", srcIP, agentHost)
	}
	if srcIP.Type != "" && dstIP.Type != "" {
		builder.relate("connected_to", srcIP, dstIP)
	}
}
