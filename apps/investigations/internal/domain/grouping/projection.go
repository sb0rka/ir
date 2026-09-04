package grouping

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

// Project consumes only the authorized raw view. Hidden evidence cannot become
// nodes, explanations, counts, or reasons in the resulting projection.
func Project(r model.ProjectionRequest, scope model.GroupScope, nodes []model.GraphNode, edges []model.GraphEdge, groups []model.Group) model.GraphProjection {
	out := model.GraphProjection{InvestigationID: r.InvestigationID, RootID: scope.RootID, HypothesisID: r.HypothesisID, IncludeSubtree: r.Filter.IncludeSubtree,
		Nodes: []model.ProjectionNode{}, Edges: []model.ProjectionEdge{}, Annotations: []model.ProjectionAnnotation{}, Diagnostics: []model.ProjectionDiagnostic{}, RawNodes: []model.ProjectionRawNode{}, RawEdges: []model.ProjectionRawEdge{}}
	out.Groups = []model.ProjectionGroup{}
	byNode := map[string]model.GraphNode{}
	byObject := map[string][]model.GraphNode{}
	visibleEvents := map[string]map[string]bool{}
	eventTimes := map[string]time.Time{}
	mapped := map[string]string{}
	for _, n := range nodes {
		byNode[n.ID] = n
		if n.EntityID != nil {
			key := "entity:" + *n.EntityID
			byObject[key] = append(byObject[key], n)
		}
		if n.EventID != nil {
			key := "event:" + *n.EventID
			byObject[key] = append(byObject[key], n)
		}
		mapped[n.ID] = n.ID
		if n.EventID != nil {
			if visibleEvents[n.InvestigationID] == nil {
				visibleEvents[n.InvestigationID] = map[string]bool{}
			}
			visibleEvents[n.InvestigationID][*n.EventID] = true
			if n.OccurredAt != nil {
				eventTimes[*n.EventID] = *n.OccurredAt
			}
		}
		out.RawNodes = append(out.RawNodes, model.ProjectionRawNode{ID: n.ID, InvestigationID: n.InvestigationID, NodeType: n.NodeType, EntityID: n.EntityID, EventID: n.EventID, Label: n.Label, TypeCode: n.TypeCode, Origin: n.Origin, SomIssueIDs: sortedUnique(n.SomIssueIDs), OccurredAt: n.OccurredAt})
	}
	// A node spanning multiple observations is collapsed only if the same owner
	// is unambiguous at every observation. Never stretch two intervals across a gap.
	observed := map[string][]time.Time{}
	for _, e := range edges {
		if _, ok := byNode[e.SourceNodeID]; !ok {
			continue
		}
		if _, ok := byNode[e.TargetNodeID]; !ok {
			continue
		}
		evidence := []string{}
		for _, id := range e.EvidenceEventIDs {
			if visibleEvents[e.InvestigationID][id] {
				evidence = append(evidence, id)
			}
		}
		out.RawEdges = append(out.RawEdges, model.ProjectionRawEdge{ID: e.ID, InvestigationID: e.InvestigationID, Source: e.SourceNodeID, Target: e.TargetNodeID, Relation: e.RelationCode, Status: e.Status, Origin: e.Origin, Confidence: e.Confidence, Why: e.Why, EvidenceEventIDs: sortedUnique(evidence)})
		for _, id := range []string{e.SourceNodeID, e.TargetNodeID} {
			for _, other := range []string{e.SourceNodeID, e.TargetNodeID} {
				if n := byNode[other]; n.OccurredAt != nil {
					observed[id] = append(observed[id], *n.OccurredAt)
				}
			}
			for _, ev := range evidence {
				if at, ok := eventTimes[ev]; ok {
					observed[id] = append(observed[id], at)
				}
			}
		}
	}
	type membership struct {
		group      model.Group
		member     model.GroupMember
		assertions []model.GroupAssertion
	}
	candidates := map[string][]membership{}
	groupNodes := map[string][]string{}
	byGroup := map[string]model.Group{}
	for _, g := range groups {
		if g.GroupScope != scope || g.State != "active" {
			continue
		}
		byGroup[g.ID] = g
		visible := model.ProjectionGroup{ID: g.ID, Family: g.Family, Kind: g.Kind, Version: g.Version, Members: []model.ProjectionGroupMember{}}
		for _, m := range g.Members {
			member := model.ProjectionGroupMember{ID: m.ID, Role: m.Role, Ordinal: m.Ordinal, Status: m.Status, Version: m.Version, Confidence: m.Confidence, NodeIDs: []string{}, Assertions: []model.GroupAssertion{}}
			for _, n := range byObject[g.Family+":"+m.ObjectID] {
				var assertions []model.GroupAssertion
				for _, a := range m.Assertions {
					if a.InvestigationID != n.InvestigationID {
						continue
					}
					if slices.ContainsFunc(a.EvidenceEventIDs, func(id string) bool { return !visibleEvents[n.InvestigationID][id] }) {
						continue
					}
					assertions = append(assertions, a)
				}
				if len(assertions) == 0 {
					continue
				}
				member.NodeIDs = append(member.NodeIDs, n.ID)
				member.Assertions = UnionAssertions(member.Assertions, assertions)
				if m.Status != "confirmed" {
					continue
				}
				if m.Role == "identifier" && n.TypeCode != nil && *n.TypeCode == "ip" {
					assertions = slices.DeleteFunc(assertions, func(a model.GroupAssertion) bool { return a.ValidFrom == nil || a.ValidTo == nil })
					if len(assertions) == 0 {
						continue
					}
				}
				candidates[n.ID] = append(candidates[n.ID], membership{g, m, assertions})
				groupNodes[g.ID] = append(groupNodes[g.ID], n.ID)
			}
			if len(member.NodeIDs) > 0 {
				member.NodeIDs = sortedUnique(member.NodeIDs)
				slices.SortFunc(member.Assertions, func(a, b model.GroupAssertion) int {
					aa, _ := json.Marshal(a)
					bb, _ := json.Marshal(b)
					return cmp.Compare(string(aa), string(bb))
				})
				visible.Members = append(visible.Members, member)
			}
		}
		if len(visible.Members) > 0 {
			slices.SortFunc(visible.Members, func(a, b model.ProjectionGroupMember) int { return cmp.Compare(a.ID, b.ID) })
			out.Groups = append(out.Groups, visible)
		}
	}
	virtual := map[string]model.ProjectionNode{}
	putGroup := func(g model.Group, ids []string) {
		id := g.Family + "-group:" + g.ID
		gid, kind := g.ID, g.Kind
		virtual[id] = model.ProjectionNode{ID: id, NodeType: g.Family + "_group", GroupID: &gid, GroupKind: &kind, MemberNodeIDs: sortedUnique(ids)}
		for _, raw := range ids {
			mapped[raw] = id
		}
	}
	diagnostic := func(code string, ids []string) {
		out.Diagnostics = append(out.Diagnostics, model.ProjectionDiagnostic{Code: code, NodeIDs: sortedUnique(ids)})
	}
	entityMembers := map[string][]string{}
	for _, n := range nodes {
		if n.NodeType != "entity" {
			continue
		}
		owners := map[string]bool{}
		timed := map[string][]model.GroupAssertion{}
		hasIdentifier := false
		for _, c := range candidates[n.ID] {
			if c.group.Family != "entity" {
				continue
			}
			// Do not use an invisible/proposed subject to hide an identifier.
			hasSubject := slices.ContainsFunc(groupNodes[c.group.ID], func(id string) bool {
				return slices.ContainsFunc(candidates[id], func(x membership) bool { return x.group.ID == c.group.ID && x.member.Role == "subject" })
			})
			if !hasSubject {
				continue
			}
			if c.member.Role == "subject" {
				owners[c.group.ID] = true
			} else {
				hasIdentifier = true
				timed[c.group.ID] = append(timed[c.group.ID], c.assertions...)
			}
		}
		if hasIdentifier {
			times := observed[n.ID]
			if len(times) == 0 {
				times = []time.Time{{}}
			}
			for _, at := range times {
				atOwners := map[string]bool{}
				for id, assertions := range timed {
					if slices.ContainsFunc(assertions, func(a model.GroupAssertion) bool { return covers(a, at) }) {
						atOwners[id] = true
						owners[id] = true
					}
				}
				if len(atOwners) != 1 {
					owners[""] = true
				}
			}
		}
		if len(owners) == 1 && !owners[""] {
			for id := range owners {
				entityMembers[id] = append(entityMembers[id], n.ID)
			}
		} else if len(owners) > 0 {
			diagnostic("ambiguous_or_unbounded_identifier", []string{n.ID})
		}
	}
	for id, ids := range entityMembers {
		putGroup(byGroup[id], ids)
	}
	// same_event has priority; inconsistent overlaps stay raw.
	sameOwners := map[string][]string{}
	for id, g := range byGroup {
		if g.Kind != "same_event" {
			continue
		}
		ids := groupNodes[id]
		hasPrimary := slices.ContainsFunc(ids, func(n string) bool {
			return slices.ContainsFunc(candidates[n], func(c membership) bool { return c.group.ID == id && c.member.Role == "primary" })
		})
		if hasPrimary {
			for _, n := range ids {
				sameOwners[n] = append(sameOwners[n], id)
			}
		}
	}
	for id, g := range byGroup {
		if g.Kind != "same_event" || len(groupNodes[id]) == 0 {
			continue
		}
		ids := groupNodes[id]
		if slices.ContainsFunc(ids, func(n string) bool { return len(sameOwners[n]) != 1 }) {
			diagnostic("same_event_overlap", ids)
			continue
		}
		putGroup(g, ids)
	}
	compositeUnits := map[string][]string{}
	unitOwners := map[string][]string{}
	for id, g := range byGroup {
		if g.Kind != "composite" {
			continue
		}
		ids := groupNodes[id]
		complete := true
		for _, n := range ids {
			unit := mapped[n]
			if v, ok := virtual[unit]; ok {
				for _, raw := range v.MemberNodeIDs {
					if !slices.Contains(ids, raw) {
						complete = false
					}
				}
			}
			compositeUnits[id] = append(compositeUnits[id], unit)
		}
		if !complete {
			diagnostic("partial_same_event_overlap", ids)
			delete(compositeUnits, id)
			continue
		}
		compositeUnits[id] = sortedUnique(compositeUnits[id])
		for _, unit := range compositeUnits[id] {
			unitOwners[unit] = append(unitOwners[unit], id)
		}
	}
	for id, units := range compositeUnits {
		if len(units) == 0 {
			continue
		}
		if slices.ContainsFunc(units, func(u string) bool { return len(unitOwners[u]) > 1 }) {
			diagnostic("composite_overlap", groupNodes[id])
			continue
		}
		putGroup(byGroup[id], groupNodes[id])
	}
	for id, g := range byGroup {
		if g.Kind != "sequence" && g.Kind != "correlation" {
			continue
		}
		ids := sortedUnique(groupNodes[id])
		if len(ids) == 0 {
			continue
		}
		if g.Kind == "sequence" {
			ordinal := func(id string) int {
				for _, c := range candidates[id] {
					if c.group.ID == g.ID && c.member.Ordinal != nil {
						return *c.member.Ordinal
					}
				}
				return 0
			}
			slices.SortFunc(ids, func(a, b string) int {
				if v := cmp.Compare(ordinal(a), ordinal(b)); v != 0 {
					return v
				}
				return cmp.Compare(a, b)
			})
		}
		out.Annotations = append(out.Annotations, model.ProjectionAnnotation{GroupID: id, Kind: g.Kind, MemberNodeIDs: ids})
	}
	used := map[string]bool{}
	for _, n := range nodes {
		id := mapped[n.ID]
		if used[id] {
			continue
		}
		used[id] = true
		if v, ok := virtual[id]; ok {
			out.Nodes = append(out.Nodes, v)
		} else {
			out.Nodes = append(out.Nodes, model.ProjectionNode{ID: id, NodeType: n.NodeType, MemberNodeIDs: []string{n.ID}})
		}
	}
	aggregates := map[string]*model.ProjectionEdge{}
	for _, e := range out.RawEdges {
		source, target := mapped[e.Source], mapped[e.Target]
		if source == target && e.Source != e.Target {
			continue
		}
		// Status remains part of the key; origins are an explicit aggregate.
		key := strings.Join([]string{source, target, e.Relation, e.Status}, "\x00")
		a := aggregates[key]
		if a == nil {
			a = &model.ProjectionEdge{ID: "edge:" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String(), Source: source, Target: target, Relation: e.Relation, Status: e.Status, Origins: []string{}, MemberEdgeIDs: []string{}, EvidenceEventIDs: []string{}}
			aggregates[key] = a
		}
		a.Origins = append(a.Origins, e.Origin)
		a.MemberEdgeIDs = append(a.MemberEdgeIDs, e.ID)
		a.EvidenceEventIDs = append(a.EvidenceEventIDs, e.EvidenceEventIDs...)
		if e.Confidence != nil {
			if a.ConfidenceMin == nil || *e.Confidence < *a.ConfidenceMin {
				a.ConfidenceMin = e.Confidence
			}
			if a.ConfidenceMax == nil || *e.Confidence > *a.ConfidenceMax {
				a.ConfidenceMax = e.Confidence
			}
		}
	}
	for _, a := range aggregates {
		a.Origins = sortedUnique(a.Origins)
		a.MemberEdgeIDs = sortedUnique(a.MemberEdgeIDs)
		a.EvidenceEventIDs = sortedUnique(a.EvidenceEventIDs)
		out.Edges = append(out.Edges, *a)
	}
	slices.SortFunc(out.Nodes, func(a, b model.ProjectionNode) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(out.Groups, func(a, b model.ProjectionGroup) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(out.Edges, func(a, b model.ProjectionEdge) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(out.RawNodes, func(a, b model.ProjectionRawNode) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(out.RawEdges, func(a, b model.ProjectionRawEdge) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(out.Annotations, func(a, b model.ProjectionAnnotation) int { return cmp.Compare(a.GroupID, b.GroupID) })
	slices.SortFunc(out.Diagnostics, func(a, b model.ProjectionDiagnostic) int {
		return cmp.Compare(a.Code+strings.Join(a.NodeIDs, ","), b.Code+strings.Join(b.NodeIDs, ","))
	})
	return out
}

func covers(a model.GroupAssertion, at time.Time) bool {
	if at.IsZero() {
		return a.ValidFrom == nil && a.ValidTo == nil
	}
	return (a.ValidFrom == nil || !at.Before(*a.ValidFrom)) && (a.ValidTo == nil || !at.After(*a.ValidTo))
}

func sortedUnique(values []string) []string {
	out := append([]string{}, values...)
	slices.Sort(out)
	return slices.Compact(out)
}
