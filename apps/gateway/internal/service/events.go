package service

import (
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
)

type SearchEventsRequest struct {
	CommonFilter   fieldmap.CommonFilter
	FilterLanguage string
	Sources        []string
	TimeFrom       time.Time
	TimeTo         time.Time
	Entities       []domain.EntityRef
	Filter         string
	Columns        []string
	Sort           []capability.EventSort
	GroupBy        []string
	GroupValues    []*string
	Limit          int
	Cursor         string
}

type SearchEventsResult struct {
	Events       []domain.Event
	Entities     []domain.Entity
	Relations    []domain.Relation
	NextCursor   string
	Total        *int64
	SourceStates []domain.SourceState
	SourceErrors []domain.SourceError
}

func filterEntities(items []domain.Entity, selected map[string]struct{}) []domain.Entity {
	result := make([]domain.Entity, 0, len(items))
	for _, item := range items {
		if _, ok := selected[entityKey(domain.EntityRef{Type: item.Type, Value: item.Value})]; ok {
			result = append(result, item)
		}
	}
	return result
}

func filterRelations(items []domain.Relation, selected map[string]struct{}) []domain.Relation {
	result := make([]domain.Relation, 0, len(items))
	for _, item := range items {
		_, sourceOK := selected[entityKey(item.SourceEntity)]
		_, targetOK := selected[entityKey(item.TargetEntity)]
		if sourceOK && targetOK {
			result = append(result, item)
		}
	}
	return result
}

func entityKey(entity domain.EntityRef) string {
	kind := strings.ToLower(strings.TrimSpace(entity.Type))
	return kind + "\x00" + domain.CanonicalValue(kind, entity.Value)
}

func sortSourceStates(items []domain.SourceState) {
	sort.Slice(items, func(i, j int) bool { return items[i].Source < items[j].Source })
}

// sumSourceTotals returns the sum of per-source totals when every non-failed
// source reported a total; otherwise nil.
func sumSourceTotals(states []domain.SourceState) *int64 {
	var sum int64
	sawSuccess := false
	for _, state := range states {
		if state.Status == "failed" {
			continue
		}
		sawSuccess = true
		if state.Total == nil {
			return nil
		}
		sum += *state.Total
	}
	if !sawSuccess {
		return nil
	}
	return &sum
}

func markCompleteStatesTruncated(items []domain.SourceState) {
	for index := range items {
		if items[index].Status == "complete" {
			items[index].Status = "truncated"
		}
	}
}
