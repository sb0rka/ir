package ptnad

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func mapSessionRow(row sessionListRow, storeID int64, timeRange TimeRange, fetchedAt time.Time) (Session, error) {
	if err := validateExternalID(row.ID); err != nil {
		return Session{}, err
	}
	start, err := parseVendorTime(row.Start)
	if err != nil {
		return Session{}, fmt.Errorf("invalid start: %w", err)
	}
	end, err := parseVendorTime(row.End)
	if err != nil {
		return Session{}, fmt.Errorf("invalid end: %w", err)
	}
	if end.Before(start) {
		return Session{}, fmt.Errorf("end precedes start")
	}
	source, err := normalizeEndpoint(Endpoint{
		IP: row.SourceIP, HostID: row.SourceHostID, DNS: row.SourceDNS,
		Port: row.SourcePort, Country: row.SourceCountry,
	})
	if err != nil {
		return Session{}, fmt.Errorf("invalid source endpoint: %w", err)
	}
	destination, err := normalizeEndpoint(Endpoint{
		IP: row.DestinationIP, HostID: row.DestinationHostID, DNS: row.DestinationDNS,
		Port: row.DestinationPort, Country: row.DestinationCountry,
	})
	if err != nil {
		return Session{}, fmt.Errorf("invalid destination endpoint: %w", err)
	}
	return Session{
		SourceRef:           sourceRef(storeID, SessionRecordType, row.ID, timeRange),
		FetchedAt:           fetchedAt,
		Start:               start,
		End:                 end,
		DurationSeconds:     end.Sub(start).Seconds(),
		Source:              source,
		Destination:         destination,
		TransportProtocol:   normalizeToken(row.Protocol),
		ApplicationProtocol: normalizeToken(row.ApplicationProtocol),
		Bytes: Counters{
			Received: row.BytesReceived,
			Sent:     row.BytesSent,
			Total:    row.BytesReceived + row.BytesSent,
		},
		State:         normalizedTokens(row.State),
		Errors:        normalizedSafeStrings(row.Errors),
		TCPFlags:      normalizedTokens(row.Flags),
		Criticality:   cloneInt64(row.Criticality),
		Severity:      sessionSeverity(row.Criticality),
		HasFiles:      row.HasFiles,
		FalsePositive: cloneBool(row.FalsePositive),
		StoreTag:      strings.TrimSpace(row.StoreTag),
	}, nil
}

func mapAttackRow(row attackListRow, storeID int64, timeRange TimeRange, fetchedAt time.Time) (Attack, error) {
	if err := validateExternalID(row.ID); err != nil {
		return Attack{}, err
	}
	if strings.TrimSpace(row.Message) == "" {
		return Attack{}, fmt.Errorf("message is required")
	}
	timestamp, err := parseVendorTime(row.Timestamp)
	if err != nil {
		return Attack{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	attacker, err := normalizeEndpoint(Endpoint{IP: row.AttackerIP, HostID: row.AttackerHostID, Country: row.AttackerCountry})
	if err != nil {
		return Attack{}, fmt.Errorf("invalid attacker endpoint: %w", err)
	}
	victim, err := normalizeEndpoint(Endpoint{IP: row.VictimIP, HostID: row.VictimHostID, Country: row.VictimCountry})
	if err != nil {
		return Attack{}, fmt.Errorf("invalid victim endpoint: %w", err)
	}
	attack := Attack{
		SourceRef:     sourceRef(storeID, AttackRecordType, row.ID, timeRange),
		FetchedAt:     fetchedAt,
		Title:         safeText(row.Message),
		Class:         safeText(row.Class),
		OccurredAt:    timestamp,
		Severity:      "unknown",
		RawPriority:   cloneInt64(row.Priority),
		SID:           row.SID,
		Revision:      row.Revision,
		FalsePositive: cloneBool(row.FalsePositive),
		Attacker:      attacker,
		Victim:        victim,
	}
	if len(row.Flows) > 0 && strings.TrimSpace(row.Flows[0].ID) != "" {
		if err := validateExternalID(row.Flows[0].ID); err != nil {
			return Attack{}, fmt.Errorf("invalid parent session ID: %w", err)
		}
		parent := sourceRef(storeID, SessionRecordType, row.Flows[0].ID, timeRange)
		attack.ParentSession = &parent
	}
	return attack, nil
}

func mapFlowDetail(detail flowDetail, storeID int64, timeRange TimeRange, fetchedAt time.Time) (Session, error) {
	if err := validateExternalID(detail.ID); err != nil {
		return Session{}, err
	}
	start, err := parseVendorTime(detail.Start)
	if err != nil {
		return Session{}, fmt.Errorf("invalid start: %w", err)
	}
	end, err := parseVendorTime(detail.End)
	if err != nil {
		return Session{}, fmt.Errorf("invalid end: %w", err)
	}
	if end.Before(start) {
		return Session{}, fmt.Errorf("end precedes start")
	}
	source, err := mapEndpoint(detail.Source)
	if err != nil {
		return Session{}, fmt.Errorf("invalid source endpoint: %w", err)
	}
	destination, err := mapEndpoint(detail.Destination)
	if err != nil {
		return Session{}, fmt.Errorf("invalid destination endpoint: %w", err)
	}
	duration := detail.Duration
	if duration == 0 {
		duration = end.Sub(start).Seconds()
	}
	session := Session{
		SourceRef:            sourceRef(storeID, SessionRecordType, detail.ID, timeRange),
		FetchedAt:            fetchedAt,
		Start:                start,
		End:                  end,
		DurationSeconds:      duration,
		Source:               source,
		Destination:          destination,
		TransportProtocol:    normalizeToken(detail.Protocol),
		ApplicationProtocol:  normalizeToken(detail.ApplicationProtocol),
		ApplicationProtocols: normalizedTokens(detail.ApplicationProtocols),
		Bytes:                mapCounters(detail.Bytes),
		Packets:              mapCounters(detail.Packets),
		State:                normalizedTokens(detail.State),
		Errors:               normalizedSafeStrings(detail.Errors),
		TCPFlags:             normalizedTokens(detail.TCP.Flags),
		TCPFlagsClient:       normalizedTokens(detail.TCP.FlagsClient),
		TCPFlagsServer:       normalizedTokens(detail.TCP.FlagsServer),
		Criticality:          cloneInt64(detail.Criticality),
		Severity:             sessionSeverity(detail.Criticality),
		HasFiles:             detail.HasFiles || len(detail.Files) > 0,
		FalsePositive:        cloneBool(detail.FalsePositive),
		StoreTag:             strings.TrimSpace(detail.StoreTag),
		StorageIndex:         strings.TrimSpace(detail.Index),
		Banners: Banners{
			Client: normalizedSafeStrings(detail.Banners.Client),
			Server: normalizedSafeStrings(detail.Banners.Server),
		},
		OperatingSystems: OperatingSystems{
			Client: normalizedStrings(detail.OperatingSystems.Client),
			Server: normalizedStrings(detail.OperatingSystems.Server),
		},
	}

	seenFiles := make(map[string]struct{}, len(detail.Files))
	for index, raw := range detail.Files {
		if err := validateChildParent("file", raw.Parent, detail.ID); err != nil {
			return Session{}, fmt.Errorf("map file %d: %w", index, err)
		}
		file, mapErr := mapFile(raw)
		if mapErr != nil {
			return Session{}, fmt.Errorf("map file %d: %w", index, mapErr)
		}
		if _, exists := seenFiles[file.ExternalID]; exists {
			continue
		}
		seenFiles[file.ExternalID] = struct{}{}
		session.Files = append(session.Files, file)
	}

	seenAttacks := make(map[string]struct{}, len(detail.Alerts))
	for index, raw := range detail.Alerts {
		if err := validateChildParent("alert", raw.Parent, detail.ID); err != nil {
			return Session{}, fmt.Errorf("map alert %d: %w", index, err)
		}
		attack, mapErr := mapAttackDetail(raw, storeID, timeRange, fetchedAt)
		if mapErr != nil {
			return Session{}, fmt.Errorf("map alert %d: %w", index, mapErr)
		}
		if _, exists := seenAttacks[attack.SourceRef.Identity()]; exists {
			continue
		}
		seenAttacks[attack.SourceRef.Identity()] = struct{}{}
		session.RelatedAttacks = append(session.RelatedAttacks, attack)
	}

	for _, raw := range detail.Credentials {
		if account := normalizeAccount(raw.Login); account != "" {
			session.Authentication = append(session.Authentication, AuthenticationHint{
				Protocol: normalizeToken(detail.ApplicationProtocol), Account: account, Valid: cloneBool(raw.Valid),
			})
		}
	}
	if err := mapProtocolHints(&session, detail); err != nil {
		return Session{}, err
	}
	session.Authentication = dedupeAuthentication(session.Authentication)
	return session, nil
}

func mapProtocolHints(session *Session, detail flowDetail) error {
	for index, raw := range detail.SSH {
		transaction, err := mapTransaction(raw.transactionDTO, detail.ID)
		if err != nil {
			return fmt.Errorf("map SSH transaction %d: %w", index, err)
		}
		session.SSH = append(session.SSH, SSHHint{
			Transaction: transaction, Authentication: normalizeToken(raw.Authentication),
			FailedPasswordCount: cloneInt64(raw.FailedPasswordCount), KeyPressed: cloneBool(raw.KeyPressed),
			Compression: normalizedStrings(raw.Compression), Encryption: normalizedStrings(raw.Encryption),
			ClientProtocol: strings.TrimSpace(raw.Client.ProtocolVersion), ClientSoftware: strings.TrimSpace(raw.Client.SoftwareVersion),
			ServerProtocol: strings.TrimSpace(raw.Server.ProtocolVersion), ServerSoftware: strings.TrimSpace(raw.Server.SoftwareVersion),
		})
	}
	for index, raw := range detail.HTTP {
		transaction, err := mapTransaction(raw.transactionDTO, detail.ID)
		if err != nil {
			return fmt.Errorf("map HTTP transaction %d: %w", index, err)
		}
		session.HTTP = append(session.HTTP, HTTPHint{
			Transaction: transaction, Method: strings.ToUpper(strings.TrimSpace(raw.Request.Method)),
			Path: sanitizeURL(raw.Request.URL), NormalizedPath: sanitizeURL(raw.Request.NormalizedURL),
			Host: normalizeHost(raw.Request.Host), Protocol: strings.TrimSpace(raw.Request.Protocol),
			RequestBytes: raw.Request.EntityLength, RequestContentType: strings.TrimSpace(raw.Request.ContentType),
			ResponseCode: raw.Response.Code, ResponseStatus: strings.TrimSpace(raw.Response.Status),
			ResponseBytes: raw.Response.EntityLength, ResponseServer: strings.TrimSpace(raw.Response.Server),
			ResponseContentType: strings.TrimSpace(raw.Response.ContentType),
		})
	}
	for index, raw := range detail.SMB {
		transaction, err := mapTransaction(raw.transactionDTO, detail.ID)
		if err != nil {
			return fmt.Errorf("map SMB transaction %d: %w", index, err)
		}
		session.SMB = append(session.SMB, SMBHint{
			Transaction: transaction,
			Command:     firstNonEmpty(raw.Request.Command, raw.Response.Command),
			Status:      strings.TrimSpace(raw.Response.Status),
			Filename:    firstNonEmpty(raw.Request.Create.Filename, raw.Response.Create.Filename),
			Action:      firstNonEmpty(raw.Response.Create.Action, raw.Request.Create.Action),
			TreePath:    firstNonEmpty(raw.Request.TreeConnect.TreePath, raw.Response.TreeConnect.TreePath),
			ShareType:   firstNonEmpty(raw.Response.TreeConnect.ShareType, raw.Request.TreeConnect.ShareType),
		})
	}
	for index, raw := range detail.DCERPC {
		transaction, err := mapTransaction(raw.transactionDTO, detail.ID)
		if err != nil {
			return fmt.Errorf("map DCERPC transaction %d: %w", index, err)
		}
		authType := firstNonEmpty(raw.Request.Auth.Type, raw.Response.Auth.Type)
		authLevel := firstNonEmpty(raw.Request.Auth.Level, raw.Response.Auth.Level)
		session.DCERPC = append(session.DCERPC, DCERPCHint{
			Transaction: transaction, PacketType: firstNonEmpty(raw.Request.PacketType, raw.Response.PacketType),
			Interface: strings.TrimSpace(raw.Request.Interface), Operation: strings.TrimSpace(raw.Request.Operation.Name),
			AuthType: strings.TrimSpace(authType), AuthLevel: strings.TrimSpace(authLevel), ArgumentsDecoded: false,
		})
	}
	for index, raw := range detail.NTLM {
		transaction, err := mapTransaction(raw.transactionDTO, detail.ID)
		if err != nil {
			return fmt.Errorf("map NTLM transaction %d: %w", index, err)
		}
		account := normalizeAccount(raw.Request.UserName)
		clientHost := normalizeHost(raw.Request.HostName)
		targetHost := normalizeHost(firstNonEmpty(raw.Request.TargetName, raw.Response.TargetName, raw.Response.TargetInfo.ServerName, raw.Response.TargetInfo.DNSHostName))
		domain := normalizeHost(firstNonEmpty(raw.Response.TargetInfo.DomainName, raw.Response.TargetInfo.DNSDomainName))
		session.NTLM = append(session.NTLM, NTLMHint{
			Transaction: transaction, MessageType: firstNonEmpty(raw.Request.MessageType, raw.Response.MessageType),
			Account: account, ClientHost: clientHost, TargetHost: targetHost, Domain: domain,
			OSVersion: firstNonEmpty(raw.Request.OSVersion, raw.Response.OSVersion),
			OSBuild:   firstNonZero(raw.Request.OSBuild, raw.Response.OSBuild),
		})
		if account != "" {
			session.Authentication = append(session.Authentication, AuthenticationHint{
				Protocol: "ntlm", Account: account, ClientHost: clientHost, ServerHost: targetHost,
			})
		}
	}
	return nil
}

func mapAttackDetail(raw alertDetail, storeID int64, timeRange TimeRange, fetchedAt time.Time) (Attack, error) {
	row := attackListRow{
		AttackerIP: raw.Attacker.IP, AttackerHostID: raw.Attacker.HostID,
		Class: raw.Class, FalsePositive: raw.FalsePositive, ID: raw.ID, Message: raw.Message,
		Priority: raw.Priority, Revision: raw.Revision, SID: raw.SID, Timestamp: raw.Timestamp,
		VictimIP: raw.Victim.IP, VictimHostID: raw.Victim.HostID,
	}
	attack, err := mapAttackRow(row, storeID, timeRange, fetchedAt)
	if err != nil {
		return Attack{}, err
	}
	attack.GID = raw.GID
	attack.Description = safeText(raw.Signature.Description.Description)
	attack.Recommendation = safeText(raw.Signature.Description.Recommendation)
	attack.RuleVendor = safeText(raw.Signature.Vendor)
	attack.AttackTarget = safeText(raw.Signature.Description.AttackTarget)
	attack.AttackFlag = cloneBool(raw.Signature.Description.AttackFlag)
	attack.RuleDisabled = cloneBool(raw.Signature.Description.Disabled)
	attack.MatchType = safeText(raw.MatchType)
	attack.ATTACK = normalizedStrings(raw.ATTACK)
	attack.Direction = direction(raw.ToServer, raw.ToClient)
	if strings.TrimSpace(raw.Parent) != "" {
		if err := validateExternalID(raw.Parent); err != nil {
			return Attack{}, fmt.Errorf("invalid parent session ID: %w", err)
		}
		parent := sourceRef(storeID, SessionRecordType, raw.Parent, timeRange)
		attack.ParentSession = &parent
	}
	return attack, nil
}

func mapFile(raw fileDetail) (FileHint, error) {
	if err := validateExternalID(raw.ID); err != nil {
		return FileHint{}, err
	}
	if strings.TrimSpace(raw.Parent) != "" {
		if err := validateExternalID(raw.Parent); err != nil {
			return FileHint{}, fmt.Errorf("invalid parent ID: %w", err)
		}
	}
	md5, err := normalizeHash(raw.MD5, 16)
	if err != nil {
		return FileHint{}, fmt.Errorf("invalid MD5: %w", err)
	}
	sha256, err := normalizeHash(raw.SHA256, 32)
	if err != nil {
		return FileHint{}, fmt.Errorf("invalid SHA-256: %w", err)
	}
	return FileHint{
		ExternalID: raw.ID, ParentID: strings.TrimSpace(raw.Parent), VendorID: raw.FileID,
		TxID: cloneInt64(raw.TxID), Name: safeText(raw.Filename), Path: safeText(raw.Filepath),
		Magic: safeText(raw.Magic), MIME: strings.ToLower(safeText(raw.MIME)),
		MD5: md5, SHA256: sha256, Size: raw.Size, State: normalizeToken(raw.State),
		Direction: direction(raw.ToServer, raw.ToClient),
	}, nil
}

func mapTransaction(raw transactionDTO, expectedParent string) (TransactionRef, error) {
	if err := validateExternalID(raw.ID); err != nil {
		return TransactionRef{}, err
	}
	if err := validateChildParent("transaction", raw.Parent, expectedParent); err != nil {
		return TransactionRef{}, err
	}
	timestamp, err := parseVendorTime(raw.Timestamp)
	if err != nil {
		return TransactionRef{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return TransactionRef{ExternalID: raw.ID, TxID: raw.TxID, OccurredAt: timestamp}, nil
}

func validateChildParent(kind, parent, expected string) error {
	if strings.TrimSpace(parent) != strings.TrimSpace(expected) {
		return fmt.Errorf("%s parent does not match the requested session", kind)
	}
	return nil
}

func mapEndpoint(raw endpointDTO) (Endpoint, error) {
	country := raw.Country
	if country == "" {
		country = raw.Geo.Country
	}
	return normalizeEndpoint(Endpoint{
		IP: raw.IP, MAC: raw.MAC, Host: raw.Name, HostID: raw.HostID,
		DNS: raw.DNS, Port: raw.Port, Country: country, Groups: raw.Groups,
	})
}

func normalizeEndpoint(endpoint Endpoint) (Endpoint, error) {
	if value := strings.TrimSpace(endpoint.IP); value != "" {
		parsed := net.ParseIP(value)
		if parsed == nil {
			return Endpoint{}, fmt.Errorf("invalid IP address")
		}
		endpoint.IP = parsed.String()
	}
	if value := strings.TrimSpace(endpoint.MAC); value != "" {
		parsed, err := net.ParseMAC(value)
		if err != nil {
			return Endpoint{}, fmt.Errorf("invalid MAC address")
		}
		parts := strings.Split(parsed.String(), ":")
		for index := range parts {
			parts[index] = strings.ToUpper(parts[index])
		}
		endpoint.MAC = strings.Join(parts, ":")
	}
	if endpoint.Port < 0 || endpoint.Port > 65535 {
		return Endpoint{}, fmt.Errorf("port is outside 0..65535")
	}
	endpoint.Host = normalizeHost(endpoint.Host)
	endpoint.HostID = strings.TrimSpace(endpoint.HostID)
	endpoint.DNS = normalizeHost(endpoint.DNS)
	endpoint.Country = strings.ToUpper(strings.TrimSpace(endpoint.Country))
	endpoint.Groups = normalizedStrings(endpoint.Groups)
	return endpoint, nil
}

func mapCounters(raw countersDTO) Counters {
	total := raw.Total
	if total == 0 && (raw.Received != 0 || raw.Sent != 0) {
		total = raw.Received + raw.Sent
	}
	return Counters{Received: raw.Received, Sent: raw.Sent, Total: total}
}

func mapStore(raw storeDetail, fetchedAt time.Time) (Store, error) {
	start, err := parseVendorTime(raw.Start)
	if err != nil {
		return Store{}, fmt.Errorf("invalid store start: %w", err)
	}
	end, err := parseVendorTime(raw.End)
	if err != nil {
		return Store{}, fmt.Errorf("invalid store end: %w", err)
	}
	lastImport, err := parseOptionalVendorTime(raw.LastImport)
	if err != nil {
		return Store{}, fmt.Errorf("invalid last import: %w", err)
	}
	created, err := parseOptionalVendorTime(raw.Created)
	if err != nil {
		return Store{}, fmt.Errorf("invalid created time: %w", err)
	}
	modified, err := parseOptionalVendorTime(raw.Modified)
	if err != nil {
		return Store{}, fmt.Errorf("invalid modified time: %w", err)
	}
	return Store{
		ID: raw.ID, Name: strings.TrimSpace(raw.Name), Description: strings.TrimSpace(raw.Description),
		Start: start, End: end, LastImport: lastImport, Created: created, Modified: modified,
		FilesCount: raw.FilesCount, Volume: raw.Volume, IsLive: raw.IsLive, FetchedAt: fetchedAt,
	}, nil
}

func sourceRef(storeID int64, recordType, externalID string, timeRange TimeRange) SourceRef {
	return SourceRef{
		SourceCode: SourceCode, SourceInstance: strconv.FormatInt(storeID, 10),
		RecordType: recordType, ExternalID: externalID, TimeRange: timeRange,
	}
}

func sessionSeverity(criticality *int64) string {
	if criticality == nil {
		return "info"
	}
	switch *criticality {
	case 1:
		return "critical"
	case 2:
		return "high"
	case 3:
		return "medium"
	case 4:
		return "low"
	default:
		return "unknown"
	}
}

func parseVendorTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("timestamp is empty")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", value, time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func parseOptionalVendorTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return parseVendorTime(value)
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return strings.ToLower(strings.TrimSuffix(value, "."))
}

func normalizeAccount(value string) string {
	value = safeText(value)
	if value == "[redacted]" {
		return ""
	}
	return strings.ToLower(value)
}

func normalizeHash(value string, bytesLength int) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != bytesLength {
		return "", fmt.Errorf("expected %d hexadecimal bytes", bytesLength)
	}
	return value, nil
}

func normalizedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizedSafeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = safeText(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return normalizedStrings(result)
}

func safeText(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"cookie=", "cookie:", "set-cookie", "authorization:", "authorization=", "proxy-authorization",
		"password=", "password:", "passwd=", "passwd:", "session_key", "nt_proof", "lm_response",
		"client_challenge", "channel_bindings",
	} {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	const maxSafeTextLength = 2048
	if len(value) > maxSafeTextLength {
		value = value[:maxSafeTextLength]
	}
	return value
}

func normalizedTokens(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range normalizedStrings(values) {
		result = append(result, normalizeToken(value))
	}
	return result
}

func dedupeAuthentication(values []AuthenticationHint) []AuthenticationHint {
	result := make([]AuthenticationHint, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := value.Protocol + "\x00" + value.Account + "\x00" + value.ClientHost + "\x00" + value.ServerHost
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sanitizeURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		if index := strings.IndexAny(value, "?#"); index >= 0 {
			value = value[:index]
		}
		return value
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	if parsed.IsAbs() {
		parsed.Scheme = ""
		parsed.Host = ""
	}
	return parsed.String()
}

func direction(toServer, toClient bool) string {
	switch {
	case toServer && !toClient:
		return "source_to_destination"
	case toClient && !toServer:
		return "destination_to_source"
	default:
		return "unknown"
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

// Stable ordering makes protocol-derived entity material deterministic even if
// a future vendor response changes array order.
func sortEndpoints(values []Endpoint) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].IP != values[j].IP {
			return values[i].IP < values[j].IP
		}
		return values[i].Port < values[j].Port
	})
}
