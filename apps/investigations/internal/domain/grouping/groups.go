// Package grouping defines pure, tree-local group transitions and projections.
package grouping

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

var (
	ErrInvalid  = errors.New("invalid group state")
	ErrConflict = errors.New("group version or decision conflict")
	ErrNotFound = errors.New("group reference not found in scope")
)

func Validate(g model.Group) error {
	if !ValidID(g.ID) || !ValidID(g.RootID) || strings.TrimSpace(g.ProjectID) == "" ||
		strings.TrimSpace(g.Title) == "" || utf8.RuneCountInString(g.Title) > 255 || len(g.Members) == 0 || len(g.Members) > 2500 ||
		(g.State != "active" && g.State != "superseded") {
		return ErrInvalid
	}
	if g.Family == "entity" {
		if g.Kind != "resolved_entity" || strings.TrimSpace(g.TypeCode) == "" {
			return ErrInvalid
		}
	} else if g.Family != "event" || g.TypeCode != "" || !slices.Contains([]string{"same_event", "composite", "sequence", "correlation"}, g.Kind) {
		return ErrInvalid
	}
	ids, objects, ordinals := map[string]bool{}, map[string]bool{}, map[int]bool{}
	active, subjects, primaries, confirmedPrimary, confirmedDuplicate, parents := 0, 0, 0, 0, 0, 0
	for _, m := range g.Members {
		if !ValidID(m.ID) || !ValidID(m.ObjectID) || ids[m.ID] || objects[m.ObjectID] || len(m.Assertions) == 0 ||
			!slices.Contains([]string{"proposed", "confirmed", "rejected"}, m.Status) {
			return ErrInvalid
		}
		ids[m.ID], objects[m.ObjectID] = true, true
		if m.Confidence != nil && (math.IsNaN(float64(*m.Confidence)) || *m.Confidence < 0 || *m.Confidence > 1) {
			return ErrInvalid
		}
		var roles []string
		switch g.Kind {
		case "resolved_entity":
			roles = []string{"subject", "identifier"}
		case "same_event":
			roles = []string{"primary", "duplicate"}
		case "composite":
			roles = []string{"parent", "part"}
		case "sequence":
			roles = []string{"step"}
		case "correlation":
			roles = []string{"evidence"}
		}
		if !slices.Contains(roles, m.Role) {
			return ErrInvalid
		}
		if g.Kind == "sequence" {
			if m.Ordinal == nil || *m.Ordinal < 0 {
				return ErrInvalid
			}
		} else if m.Ordinal != nil {
			return ErrInvalid
		}
		for _, a := range m.Assertions {
			if !ValidID(a.InvestigationID) || !slices.Contains([]string{"source", "rule", "analyst", "agent"}, a.Origin) || a.Method == "" || a.MethodVersion == "" ||
				(a.ValidFrom != nil && a.ValidTo != nil && a.ValidTo.Before(*a.ValidFrom)) {
				return ErrInvalid
			}
			for _, id := range a.EvidenceEventIDs {
				if !ValidID(id) {
					return ErrInvalid
				}
			}
		}
		if m.Status == "rejected" {
			continue
		}
		active++
		if m.Role == "subject" {
			subjects++
		}
		if m.Role == "primary" {
			primaries++
			if m.Status == "confirmed" {
				confirmedPrimary++
			}
		}
		if m.Role == "duplicate" && m.Status == "confirmed" {
			confirmedDuplicate++
		}
		if m.Role == "parent" {
			parents++
		}
		if m.Ordinal != nil {
			if ordinals[*m.Ordinal] {
				return ErrInvalid
			}
			ordinals[*m.Ordinal] = true
		}
	}
	if active > 0 && ((g.Kind == "resolved_entity" && subjects == 0) || (g.Kind == "same_event" && primaries != 1)) ||
		confirmedDuplicate > 0 && confirmedPrimary != 1 || parents > 1 {
		return ErrInvalid
	}
	return nil
}

func ValidID(s string) bool { v, err := uuid.Parse(s); return err == nil && v != uuid.Nil }

func Clone(g model.Group) model.Group {
	g.Members = slices.Clone(g.Members)
	g.SuccessorIDs = slices.Clone(g.SuccessorIDs)
	for i := range g.Members {
		g.Members[i].Assertions = slices.Clone(g.Members[i].Assertions)
		for j := range g.Members[i].Assertions {
			g.Members[i].Assertions[j].EvidenceEventIDs = slices.Clone(g.Members[i].Assertions[j].EvidenceEventIDs)
		}
	}
	return g
}

func Review(g model.Group, r model.GroupReview) (model.Group, error) {
	if g.State != "active" || g.Version != r.Version {
		return g, ErrConflict
	}
	if !ValidID(r.OperationID) || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 4096 || len(r.Members) == 0 || len(r.Members) > 2500 {
		return g, ErrInvalid
	}
	out := Clone(g)
	seen := map[string]bool{}
	for _, item := range r.Members {
		if seen[item.ID] || !slices.Contains([]string{"confirmed", "rejected"}, item.Status) {
			return g, ErrInvalid
		}
		seen[item.ID] = true
		index := slices.IndexFunc(out.Members, func(m model.GroupMember) bool { return m.ID == item.ID })
		if index < 0 {
			return g, ErrNotFound
		}
		m := &out.Members[index]
		if m.Version != item.Version {
			return g, ErrConflict
		}
		m.Status, m.Reason, m.Version = item.Status, r.Reason, m.Version+1
	}
	out.Version++
	return out, Validate(out)
}

func Merge(target model.Group, sources []model.Group, r model.GroupMerge) (model.Group, error) {
	if target.State != "active" || target.Version != r.Version {
		return target, ErrConflict
	}
	if !ValidID(r.OperationID) || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 4096 || len(sources) == 0 || len(sources) > 100 {
		return target, ErrInvalid
	}
	all := append([]model.Group{target}, sources...)
	byID := map[string]model.GroupMember{}
	targetObjects := map[string]model.GroupMember{}
	for _, m := range target.Members {
		targetObjects[m.ObjectID] = m
	}
	for _, g := range all {
		if g.GroupScope != target.GroupScope {
			return target, ErrNotFound
		}
		if g.State != "active" || g.Family != target.Family || g.Kind != target.Kind || g.TypeCode != target.TypeCode {
			return target, ErrConflict
		}
		for _, m := range g.Members {
			byID[m.ID] = m
		}
	}
	if len(r.Members) != len(byID) || len(r.Members) > 2500 {
		return target, ErrInvalid
	}
	out := Clone(target)
	out.Members = nil
	seen, byObject := map[string]bool{}, map[string]int{}
	for _, placement := range r.Members {
		m, ok := byID[placement.MemberID]
		if !ok {
			return target, ErrNotFound
		}
		if seen[m.ID] {
			return target, ErrInvalid
		}
		seen[m.ID] = true
		m.Role, m.Ordinal = placement.Role, placement.Ordinal
		m.Assertions = slices.Clone(m.Assertions)
		if i, exists := byObject[m.ObjectID]; exists {
			old := &out.Members[i]
			if old.Status != m.Status || old.Role != m.Role || !equalOrdinal(old.Ordinal, m.Ordinal) {
				return target, ErrConflict
			}
			old.Assertions = UnionAssertions(old.Assertions, m.Assertions)
			// Different confidence observations are retained conservatively.
			if old.Confidence == nil || m.Confidence == nil {
				old.Confidence = nil
			} else if *m.Confidence < *old.Confidence {
				old.Confidence = m.Confidence
			}
		} else {
			// New survivor membership IDs avoid moving historical source membership rows.
			if original, ok := targetObjects[m.ObjectID]; ok {
				m.ID = original.ID
				m.Version = original.Version
			} else {
				m.ID = uuid.NewString()
				m.Version = 0
			}
			m.Version++
			m.Reason = r.Reason
			byObject[m.ObjectID] = len(out.Members)
			out.Members = append(out.Members, m)
		}
	}
	out.Version++
	return out, Validate(out)
}

func Split(g model.Group, r model.GroupSplit) ([]model.Group, error) {
	if g.State != "active" || g.Version != r.Version {
		return nil, ErrConflict
	}
	if !ValidID(r.OperationID) || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 4096 || len(r.Partitions) < 2 || len(r.Partitions) > 100 {
		return nil, ErrInvalid
	}
	byID, seen := map[string]model.GroupMember{}, map[string]int{}
	for _, m := range g.Members {
		byID[m.ID] = m
	}
	result := make([]model.Group, 0, len(r.Partitions))
	for _, part := range r.Partitions {
		out := Clone(g)
		out.ID = uuid.NewString()
		out.Key = "split:" + out.ID
		out.Title = part.Title
		out.Version = 1
		out.Members = nil
		out.SuccessorIDs = []string{}
		for _, p := range part.Members {
			m, ok := byID[p.MemberID]
			if !ok {
				return nil, ErrNotFound
			}
			seen[m.ID]++
			if seen[m.ID] > 1 && !(g.Family == "entity" && m.Role == "identifier" && p.Role == "identifier") {
				return nil, ErrInvalid
			}
			m.ID = uuid.NewString()
			m.Role = p.Role
			m.Ordinal = p.Ordinal
			m.Version = 1
			m.Reason = r.Reason
			out.Members = append(out.Members, m)
		}
		if err := Validate(out); err != nil {
			return nil, err
		}
		result = append(result, out)
	}
	if len(seen) != len(byID) {
		return nil, ErrInvalid
	}
	return result, nil
}

func UnionAssertions(a, b []model.GroupAssertion) []model.GroupAssertion {
	result := slices.Clone(a)
	seen := map[string]bool{}
	for _, v := range a {
		raw, _ := json.Marshal(v)
		seen[string(raw)] = true
	}
	for _, v := range b {
		raw, _ := json.Marshal(v)
		if !seen[string(raw)] {
			result = append(result, v)
			seen[string(raw)] = true
		}
	}
	return result
}

func equalOrdinal(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
