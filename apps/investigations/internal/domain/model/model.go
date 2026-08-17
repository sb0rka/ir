// Package model — доменные типы. Зависимостей от транспорта и стора нет.
package model

import "time"

// Вокабуляр ролей свой: с платформенными owner/admin/editor/viewer не совпадает.
type SubjectRoles struct {
	ProjectID string
	Roles     []string
}

// InvestigationNew — поля, которые задаёт создатель. Остальное (status,
// version, origin) назначает сервер.
type InvestigationNew struct {
	ProjectID   string
	Title       string
	Description *string
	Severity    *string
	ParentID    *string
	// Схема хранит одну ссылку на SOM workspace; контракт говорит массивом —
	// первый элемент попадает сюда, остальные для демо отбрасываются.
	WorkspaceID *string
}

type Investigation struct {
	ID          string
	ProjectID   string
	ParentID    *string
	WorkspaceID *string
	Title       string
	Description *string
	Status      string
	Severity    *string
	Origin      string
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Counters    InvestigationCounters
}

type InvestigationCounters struct {
	Children      int
	Events        int
	Entities      int
	ProposedEdges int
}

// InvestigationFilter — фильтры списка. Nil-поле = «не фильтровать».
// ParentID == nil означает только корни (parent_id IS NULL); чтобы взять
// детей, передайте ParentID с конкретным id.
type InvestigationFilter struct {
	ParentID *string
	RootsOnly bool
	Status   *string
	Severity *string
	Q        *string
	Limit    int
}

// NodeFilter — фильтры списка нод. Limit == 0 означает «без лимита»
// (нужно getGraph).
type NodeFilter struct {
	NodeType *string
	Q        *string
	Limit    int
}

// EventIngest — событие, готовое к записи: нормализация уже выполнена
// вызывающим (сервером), стор только сохраняет.
type EventIngest struct {
	SourceCode     string
	SourceEventID  string
	SourceRef      *string
	EventType      string
	OccurredAt     time.Time
	NormalizedData []byte
	RawData        []byte
	DedupKey       string
}

// EventRef адресует запись источника — то, чем оперирует attachEvents.refs.
type EventRef struct {
	SourceCode    string
	SourceEventID string
}

// EventFilter — фильтры таймлайна. Nil-поле означает «не фильтровать».
type EventFilter struct {
	EventType  *string
	SourceCode *string
	From       *time.Time
	To         *time.Time
	Limit      int
}

// EventSummary — событие вместе со свойствами его привязки к расследованию.
type EventSummary struct {
	ID             string
	SourceCode     string
	SourceEventID  string
	SourceRef      *string
	EventType      string
	OccurredAt     time.Time
	IngestedAt     time.Time
	AttachedAt     time.Time
	AttachedBy     string
	Reason         *string
	NormalizedData []byte
}

// AttachStats — что изменил pull: слагаемые ответа EventAttachResult.
type AttachStats struct {
	Attached   int // новые для расследования события, засосанные сейчас
	Reused     int // существующие события проекта, впервые привязанные
	Duplicates int // уже были привязаны — пропущены
}

type GraphNode struct {
	ID              string
	InvestigationID string
	NodeType        string
	EntityID        *string
	EventID         *string
	Origin          string
	SomIssueIDs     []string
	// Для event-нод: подпись и время из самого события.
	Label      *string
	OccurredAt *time.Time
}
