package wazuh

import "strings"

type fieldType string

const (
	fieldKeyword fieldType = "keyword"
	fieldLong    fieldType = "long"
	fieldDate    fieldType = "date"
	fieldIP      fieldType = "ip"
)

type fieldSpec struct {
	Path            string
	Type            fieldType
	CaseInsensitive bool
	// CorrelationName requires exists rule.frequency.
	CorrelationName bool
	// CorrelationType is synthetic.
	CorrelationType bool
	// DocumentID maps to _id.
	DocumentID bool
}

// fieldCatalog is Wazuh-native only (plus canonical time/uuid/correlation_*).
// MaxPatrol PDQL aliases are intentionally not registered here.
var fieldCatalog = map[string]fieldSpec{
	"uuid":             {Path: "_id", Type: fieldKeyword, DocumentID: true},
	"correlation_name": {Path: "rule.description", Type: fieldKeyword, CorrelationName: true},
	"correlation_type": {Type: fieldKeyword, CorrelationType: true},
	"time":             {Path: "@timestamp", Type: fieldDate},

	"rule.id":              {Path: "rule.id", Type: fieldKeyword},
	"rule.level":           {Path: "rule.level", Type: fieldLong},
	"rule.groups":          {Path: "rule.groups", Type: fieldKeyword},
	"rule.description":     {Path: "rule.description", Type: fieldKeyword},
	"rule.firedtimes":      {Path: "rule.firedtimes", Type: fieldLong},
	"rule.mitre.id":        {Path: "rule.mitre.id", Type: fieldKeyword},
	"rule.mitre.tactic":    {Path: "rule.mitre.tactic", Type: fieldKeyword},
	"rule.mitre.technique": {Path: "rule.mitre.technique", Type: fieldKeyword},
	"rule.frequency":       {Path: "rule.frequency", Type: fieldLong},

	"agent.id":     {Path: "agent.id", Type: fieldKeyword},
	"agent.name":   {Path: "agent.name", Type: fieldKeyword, CaseInsensitive: true},
	"agent.ip":     {Path: "agent.ip", Type: fieldKeyword},
	"manager.name": {Path: "manager.name", Type: fieldKeyword},
	"decoder.name": {Path: "decoder.name", Type: fieldKeyword},
	"location":     {Path: "location", Type: fieldKeyword},

	"data.srcip":       {Path: "data.srcip", Type: fieldKeyword},
	"data.dstip":       {Path: "data.dstip", Type: fieldKeyword},
	"data.srcport":     {Path: "data.srcport", Type: fieldKeyword},
	"data.dstport":     {Path: "data.dstport", Type: fieldKeyword},
	"data.srcuser":     {Path: "data.srcuser", Type: fieldKeyword, CaseInsensitive: true},
	"data.dstuser":     {Path: "data.dstuser", Type: fieldKeyword, CaseInsensitive: true},
	"data.protocol":    {Path: "data.protocol", Type: fieldKeyword},
	"data.url":         {Path: "data.url", Type: fieldKeyword},
	"data.id":          {Path: "data.id", Type: fieldKeyword},
	"data.status":      {Path: "data.status", Type: fieldKeyword},
	"data.system_name": {Path: "data.system_name", Type: fieldKeyword, CaseInsensitive: true},
	"data.integration": {Path: "data.integration", Type: fieldKeyword},

	"data.win.system.eventID":               {Path: "data.win.system.eventID", Type: fieldKeyword},
	"data.win.system.computer":              {Path: "data.win.system.computer", Type: fieldKeyword, CaseInsensitive: true},
	"data.win.system.channel":               {Path: "data.win.system.channel", Type: fieldKeyword},
	"data.win.eventdata.targetUserName":     {Path: "data.win.eventdata.targetUserName", Type: fieldKeyword, CaseInsensitive: true},
	"data.win.eventdata.ipAddress":          {Path: "data.win.eventdata.ipAddress", Type: fieldKeyword},
	"data.win.eventdata.logonType":          {Path: "data.win.eventdata.logonType", Type: fieldKeyword},

	"syscheck.path":         {Path: "syscheck.path", Type: fieldKeyword},
	"syscheck.event":        {Path: "syscheck.event", Type: fieldKeyword},
	"syscheck.md5_after":    {Path: "syscheck.md5_after", Type: fieldKeyword},
	"syscheck.sha1_after":   {Path: "syscheck.sha1_after", Type: fieldKeyword},
	"syscheck.sha256_after": {Path: "syscheck.sha256_after", Type: fieldKeyword},

	"data.vulnerability.cve":                  {Path: "data.vulnerability.cve", Type: fieldKeyword},
	"data.vulnerability.severity":             {Path: "data.vulnerability.severity", Type: fieldKeyword},
	"data.vulnerability.package.name":         {Path: "data.vulnerability.package.name", Type: fieldKeyword},
	"data.vulnerability.package.version":      {Path: "data.vulnerability.package.version", Type: fieldKeyword},
	"data.vulnerability.status":               {Path: "data.vulnerability.status", Type: fieldKeyword},
	"data.vulnerability.cvss.cvss3.base_score": {Path: "data.vulnerability.cvss.cvss3.base_score", Type: fieldKeyword},

	"data.virustotal.source.md5":  {Path: "data.virustotal.source.md5", Type: fieldKeyword},
	"data.virustotal.source.sha1": {Path: "data.virustotal.source.sha1", Type: fieldKeyword},
	"data.virustotal.source.file": {Path: "data.virustotal.source.file", Type: fieldKeyword},
	"data.virustotal.positives":   {Path: "data.virustotal.positives", Type: fieldKeyword},

	"data.aws.source":    {Path: "data.aws.source", Type: fieldKeyword},
	"data.aws.accountId": {Path: "data.aws.accountId", Type: fieldKeyword},
	"data.aws.region":    {Path: "data.aws.region", Type: fieldKeyword},
	"data.aws.type":      {Path: "data.aws.type", Type: fieldKeyword},
	"data.aws.title":     {Path: "data.aws.title", Type: fieldKeyword},

	"data.office365.UserId":    {Path: "data.office365.UserId", Type: fieldKeyword, CaseInsensitive: true},
	"data.office365.ClientIP":  {Path: "data.office365.ClientIP", Type: fieldKeyword},
	"data.office365.Operation": {Path: "data.office365.Operation", Type: fieldKeyword},
	"data.office365.Workload":  {Path: "data.office365.Workload", Type: fieldKeyword},

	"data.github.actor":  {Path: "data.github.actor", Type: fieldKeyword, CaseInsensitive: true},
	"data.github.org":    {Path: "data.github.org", Type: fieldKeyword},
	"data.github.action": {Path: "data.github.action", Type: fieldKeyword},
	"data.github.repo":   {Path: "data.github.repo", Type: fieldKeyword},

	"data.gcp.jsonPayload.sourceIP":       {Path: "data.gcp.jsonPayload.sourceIP", Type: fieldKeyword},
	"data.gcp.jsonPayload.vmInstanceName": {Path: "data.gcp.jsonPayload.vmInstanceName", Type: fieldKeyword, CaseInsensitive: true},
	"data.gcp.resource.labels.project_id": {Path: "data.gcp.resource.labels.project_id", Type: fieldKeyword},

	"data.audit.command": {Path: "data.audit.command", Type: fieldKeyword},
	"data.audit.exe":     {Path: "data.audit.exe", Type: fieldKeyword},
	"data.audit.success": {Path: "data.audit.success", Type: fieldKeyword},
	"data.audit.type":    {Path: "data.audit.type", Type: fieldKeyword},

	"data.docker.Action": {Path: "data.docker.Action", Type: fieldKeyword},
	"data.docker.Type":   {Path: "data.docker.Type", Type: fieldKeyword},

	"data.osquery.name":   {Path: "data.osquery.name", Type: fieldKeyword},
	"data.osquery.action": {Path: "data.osquery.action", Type: fieldKeyword},
	"data.osquery.pack":   {Path: "data.osquery.pack", Type: fieldKeyword},

	"GeoLocation.country_name": {Path: "GeoLocation.country_name", Type: fieldKeyword},
	"GeoLocation.city_name":    {Path: "GeoLocation.city_name", Type: fieldKeyword},

	"data.ms-graph.ipAddress": {Path: "data.ms-graph.ipAddress", Type: fieldKeyword},
	"data.ms-graph.title":     {Path: "data.ms-graph.title", Type: fieldKeyword},
	"data.ms-graph.severity":  {Path: "data.ms-graph.severity", Type: fieldKeyword},
	"data.ms-graph.category":  {Path: "data.ms-graph.category", Type: fieldKeyword},
}

var sourceIncludes = []string{
	"@timestamp", "timestamp", "id", "location",
	"agent.id", "agent.name", "agent.ip",
	"manager.name", "decoder.name",
	"rule.id", "rule.level", "rule.description", "rule.groups", "rule.firedtimes", "rule.frequency",
	"rule.mitre.id", "rule.mitre.tactic", "rule.mitre.technique",
	"data.srcip", "data.dstip", "data.srcport", "data.dstport", "data.srcuser", "data.dstuser",
	"data.protocol", "data.url", "data.id", "data.status", "data.system_name", "data.title",
	"data.file", "data.command", "data.integration",
	"data.win.eventdata.targetUserName", "data.win.eventdata.ipAddress", "data.win.eventdata.logonType",
	"data.win.eventdata.processId", "data.win.system.computer", "data.win.system.eventID", "data.win.system.channel",
	"data.aws.source", "data.aws.accountId", "data.aws.region", "data.aws.type", "data.aws.title",
	"data.aws.service.action.actionType",
	"data.aws.service.action.networkConnectionAction.connectionDirection",
	"data.aws.service.action.networkConnectionAction.localIpDetails.ipAddressV4",
	"data.aws.service.action.networkConnectionAction.remoteIpDetails.ipAddressV4",
	"data.aws.service.action.awsApiCallAction.remoteIpDetails.ipAddressV4",
	"data.office365.UserId", "data.office365.ClientIP", "data.office365.Operation", "data.office365.Workload",
	"data.github.actor", "data.github.user", "data.github.org", "data.github.repo", "data.github.action",
	"data.gcp.jsonPayload.sourceIP", "data.gcp.jsonPayload.vmInstanceName", "data.gcp.jsonPayload.queryName",
	"data.gcp.resource.labels.project_id",
	"data.virustotal.positives", "data.virustotal.source.md5", "data.virustotal.source.sha1", "data.virustotal.source.file",
	"data.vulnerability.cve", "data.vulnerability.severity", "data.vulnerability.status", "data.vulnerability.title",
	"data.vulnerability.package.name", "data.vulnerability.package.version",
	"data.vulnerability.cvss.cvss3.base_score",
	"data.audit.command", "data.audit.exe", "data.audit.success", "data.audit.type", "data.audit.file.name",
	"data.docker.Action", "data.docker.Type",
	"data.osquery.name", "data.osquery.action", "data.osquery.pack",
	"data.ms-graph.ipAddress", "data.ms-graph.title", "data.ms-graph.severity", "data.ms-graph.category",
	"syscheck.path", "syscheck.event", "syscheck.md5_after", "syscheck.sha1_after", "syscheck.sha256_after",
	"GeoLocation.country_name", "GeoLocation.city_name",
}

func lookupField(name string) (fieldSpec, bool) {
	spec, ok := fieldCatalog[strings.TrimSpace(name)]
	return spec, ok
}
