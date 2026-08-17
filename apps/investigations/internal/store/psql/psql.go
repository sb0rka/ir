package psql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	corestore "github.com/sb0rka/sb0rka/packages/core/store"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

// Пул, Ping и Close живут в core: подключение одинаково во всех сервисах.
type DB struct {
	*corestore.Pool
}

func New(_ context.Context, uri string, maxConns int, connMaxLifetime time.Duration) (*DB, error) {
	pool, err := corestore.NewPool(uri, maxConns, connMaxLifetime)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) RoleBindings(ctx context.Context, subjectID, projectID string) (model.SubjectRoles, error) {
	rows, err := d.Pgx().Query(ctx, `
		SELECT role
		  FROM role_bindings
		 WHERE subject_id = $1
		   AND project_id = $2
		 ORDER BY role
	`, subjectID, projectID)
	if err != nil {
		return model.SubjectRoles{}, fmt.Errorf("query role bindings: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return model.SubjectRoles{}, fmt.Errorf("scan role binding: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return model.SubjectRoles{}, fmt.Errorf("iterate role bindings: %w", err)
	}

	return model.SubjectRoles{ProjectID: projectID, Roles: roles}, nil
}

const pgForeignKeyViolation = "23503"
const pgCheckViolation = "23514"
const pgInvalidTextRepresentation = "22P02"

func (d *DB) CreateInvestigation(ctx context.Context, inv model.InvestigationNew) (model.Investigation, error) {
	row := d.Pgx().QueryRow(ctx, `
		INSERT INTO investigations (project_id, workspace_id, title, description, severity, parent_id, origin)
		VALUES ($1, $2::uuid, $3, $4, $5, $6::uuid, 'analyst')
		RETURNING id::text, project_id, parent_id::text, workspace_id::text,
		          title, description, status, severity, origin, version, created_at, updated_at
	`, inv.ProjectID, inv.WorkspaceID, inv.Title, inv.Description, inv.Severity, inv.ParentID)

	var out model.Investigation
	err := row.Scan(&out.ID, &out.ProjectID, &out.ParentID, &out.WorkspaceID,
		&out.Title, &out.Description, &out.Status, &out.Severity, &out.Origin,
		&out.Version, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgForeignKeyViolation:
				return model.Investigation{}, store.ErrParentNotFound
			case pgCheckViolation, pgInvalidTextRepresentation:
				return model.Investigation{}, store.ErrInvalidValue
			}
		}
		return model.Investigation{}, fmt.Errorf("insert investigation: %w", err)
	}
	return out, nil
}

func (d *DB) ListInvestigations(ctx context.Context, projectID string, filter model.InvestigationFilter) ([]model.Investigation, error) {
	var parentID any
	if filter.ParentID != nil {
		parentID = *filter.ParentID
	}

	rows, err := d.Pgx().Query(ctx, `
		SELECT i.id::text, i.project_id, i.parent_id::text, i.workspace_id::text,
		       i.title, i.description, i.status, i.severity, i.origin, i.version,
		       i.created_at, i.updated_at,
		       (SELECT count(*)::int FROM investigations c WHERE c.parent_id = i.id) AS children,
		       (SELECT count(*)::int FROM investigation_events ie WHERE ie.investigation_id = i.id) AS events,
		       (SELECT count(*)::int FROM investigation_entities ient WHERE ient.investigation_id = i.id) AS entities,
		       (SELECT count(*)::int FROM edges e WHERE e.investigation_id = i.id AND e.status = 'proposed') AS proposed_edges
		  FROM investigations i
		 WHERE i.project_id = $1
		   AND (
		         ($2::boolean AND i.parent_id IS NULL)
		      OR ($3::uuid IS NOT NULL AND i.parent_id = $3::uuid)
		      OR (NOT $2::boolean AND $3::uuid IS NULL)
		       )
		   AND ($4::text IS NULL OR i.status = $4)
		   AND ($5::text IS NULL OR i.severity = $5)
		   AND ($6::text IS NULL OR i.title ILIKE '%' || $6 || '%')
		 ORDER BY i.created_at DESC, i.id DESC
		 LIMIT $7
	`, projectID, filter.RootsOnly, parentID, filter.Status, filter.Severity, filter.Q, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list investigations: %w", err)
	}
	defer rows.Close()

	var items []model.Investigation
	for rows.Next() {
		var item model.Investigation
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.ParentID, &item.WorkspaceID,
			&item.Title, &item.Description, &item.Status, &item.Severity, &item.Origin,
			&item.Version, &item.CreatedAt, &item.UpdatedAt,
			&item.Counters.Children, &item.Counters.Events,
			&item.Counters.Entities, &item.Counters.ProposedEdges,
		); err != nil {
			return nil, fmt.Errorf("scan investigation: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) InvestigationExists(ctx context.Context, projectID, investigationID string) (bool, error) {
	var one int
	err := d.Pgx().QueryRow(ctx, `
		SELECT 1 FROM investigations WHERE id = $1::uuid AND project_id = $2
	`, investigationID, projectID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check investigation: %w", err)
	}
	return true, nil
}

func (d *DB) AttachEvents(ctx context.Context, projectID, investigationID string,
	events []model.EventIngest, attachedBy string, reason *string) (model.AttachStats, error) {

	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.AttachStats{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exists, err := investigationExistsTx(ctx, tx, projectID, investigationID)
	if err != nil {
		return model.AttachStats{}, err
	}
	if !exists {
		return model.AttachStats{}, store.ErrInvestigationNotFound
	}

	var stats model.AttachStats
	for _, event := range events {
		var eventID string
		var inserted bool
		// DO UPDATE вместо DO NOTHING: иначе RETURNING не отдаёт id уже
		// существующей записи и пришлось бы делать второй запрос.
		err := tx.QueryRow(ctx, `
			INSERT INTO events (project_id, source_code, source_event_id, source_ref,
			                    event_type, occurred_at, normalized_data, raw_data, dedup_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9)
			ON CONFLICT (project_id, source_code, source_event_id)
			DO UPDATE SET source_ref = COALESCE(EXCLUDED.source_ref, events.source_ref)
			RETURNING id::text, (xmax = 0) AS inserted
		`, projectID, event.SourceCode, event.SourceEventID, event.SourceRef,
			event.EventType, event.OccurredAt, string(event.NormalizedData),
			nullableJSON(event.RawData), event.DedupKey).Scan(&eventID, &inserted)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
				return model.AttachStats{}, store.ErrUnknownSource
			}
			return model.AttachStats{}, fmt.Errorf("upsert event %s/%s: %w", event.SourceCode, event.SourceEventID, err)
		}

		linked, err := linkEventTx(ctx, tx, projectID, investigationID, eventID, attachedBy, reason)
		if err != nil {
			return model.AttachStats{}, err
		}
		switch {
		case !linked:
			stats.Duplicates++
		case inserted:
			stats.Attached++
		default:
			stats.Reused++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.AttachStats{}, fmt.Errorf("commit: %w", err)
	}
	return stats, nil
}

func (d *DB) FindEventIDs(ctx context.Context, projectID string, refs []model.EventRef) (map[model.EventRef]string, error) {
	sourceCodes := make([]string, len(refs))
	sourceEventIDs := make([]string, len(refs))
	for i, ref := range refs {
		sourceCodes[i] = ref.SourceCode
		sourceEventIDs[i] = ref.SourceEventID
	}

	rows, err := d.Pgx().Query(ctx, `
		SELECT e.source_code, e.source_event_id, e.id::text
		  FROM events e
		  JOIN unnest($2::text[], $3::text[]) AS r(source_code, source_event_id)
		    ON r.source_code = e.source_code AND r.source_event_id = e.source_event_id
		 WHERE e.project_id = $1
	`, projectID, sourceCodes, sourceEventIDs)
	if err != nil {
		return nil, fmt.Errorf("find events by refs: %w", err)
	}
	defer rows.Close()

	found := make(map[model.EventRef]string, len(refs))
	for rows.Next() {
		var ref model.EventRef
		var id string
		if err := rows.Scan(&ref.SourceCode, &ref.SourceEventID, &id); err != nil {
			return nil, fmt.Errorf("scan event ref: %w", err)
		}
		found[ref] = id
	}
	return found, rows.Err()
}

func (d *DB) InvestigationEvents(ctx context.Context, projectID, investigationID string,
	filter model.EventFilter) ([]model.EventSummary, error) {

	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, store.ErrInvestigationNotFound
	}

	rows, err := d.Pgx().Query(ctx, `
		SELECT e.id::text, e.source_code, e.source_event_id, e.source_ref,
		       e.event_type, e.occurred_at, e.ingested_at,
		       ie.attached_at, ie.attached_by, ie.reason, e.normalized_data
		  FROM investigation_events ie
		  JOIN events e ON e.id = ie.event_id
		 WHERE ie.investigation_id = $1::uuid
		   AND ie.project_id = $2
		   AND ($3::text IS NULL OR e.event_type = $3)
		   AND ($4::text IS NULL OR e.source_code = $4)
		   AND ($5::timestamptz IS NULL OR e.occurred_at >= $5)
		   AND ($6::timestamptz IS NULL OR e.occurred_at < $6)
		 ORDER BY e.occurred_at, e.id
		 LIMIT $7
	`, investigationID, projectID, filter.EventType, filter.SourceCode,
		filter.From, filter.To, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("query investigation events: %w", err)
	}
	defer rows.Close()

	var items []model.EventSummary
	for rows.Next() {
		var item model.EventSummary
		if err := rows.Scan(&item.ID, &item.SourceCode, &item.SourceEventID, &item.SourceRef,
			&item.EventType, &item.OccurredAt, &item.IngestedAt,
			&item.AttachedAt, &item.AttachedBy, &item.Reason, &item.NormalizedData); err != nil {
			return nil, fmt.Errorf("scan investigation event: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) LinkEvents(ctx context.Context, projectID, investigationID string,
	eventIDs []string, attachedBy string, reason *string) (int, int, error) {

	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exists, err := investigationExistsTx(ctx, tx, projectID, investigationID)
	if err != nil {
		return 0, 0, err
	}
	if !exists {
		return 0, 0, store.ErrInvestigationNotFound
	}

	linked := 0
	for _, eventID := range eventIDs {
		ok, err := linkEventTx(ctx, tx, projectID, investigationID, eventID, attachedBy, reason)
		if err != nil {
			return 0, 0, err
		}
		if ok {
			linked++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}
	return linked, len(eventIDs) - linked, nil
}

func (d *DB) CreateNode(ctx context.Context, projectID, investigationID, nodeType string,
	entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, error) {

	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.GraphNode{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exists, err := investigationExistsTx(ctx, tx, projectID, investigationID)
	if err != nil {
		return model.GraphNode{}, err
	}
	if !exists {
		return model.GraphNode{}, store.ErrInvestigationNotFound
	}

	// ON CONFLICT DO NOTHING без цели покрывает оба частичных уникальных
	// индекса; пустой RETURNING означает, что нода уже есть — берём её.
	var nodeID string
	err = tx.QueryRow(ctx, `
		INSERT INTO graph_nodes (investigation_id, node_type, entity_id, event_id, origin)
		VALUES ($1::uuid, $2, $3::uuid, $4::uuid, $5)
		ON CONFLICT DO NOTHING
		RETURNING id::text
	`, investigationID, nodeType, entityID, eventID, origin).Scan(&nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM graph_nodes
			 WHERE investigation_id = $1::uuid
			   AND ($2::uuid IS NOT NULL AND entity_id = $2::uuid
			     OR $3::uuid IS NOT NULL AND event_id = $3::uuid)
		`, investigationID, entityID, eventID).Scan(&nodeID)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case pgForeignKeyViolation:
				return model.GraphNode{}, store.ErrTargetNotAttached
			case pgCheckViolation, pgInvalidTextRepresentation:
				return model.GraphNode{}, store.ErrInvalidValue
			}
		}
		return model.GraphNode{}, fmt.Errorf("insert graph node: %w", err)
	}

	if len(somIssueIDs) > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO graph_node_som_issues (graph_node_id, som_issue_id)
			SELECT $1::uuid, unnest($2::uuid[])
			ON CONFLICT DO NOTHING
		`, nodeID, somIssueIDs)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgInvalidTextRepresentation {
				return model.GraphNode{}, store.ErrInvalidValue
			}
			return model.GraphNode{}, fmt.Errorf("link som issues: %w", err)
		}
	}

	node, err := graphNodeTx(ctx, tx, nodeID)
	if err != nil {
		return model.GraphNode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.GraphNode{}, fmt.Errorf("commit: %w", err)
	}
	return node, nil
}

func (d *DB) GraphNodes(ctx context.Context, projectID, investigationID string, filter model.NodeFilter) ([]model.GraphNode, error) {
	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, store.ErrInvestigationNotFound
	}

	limit := filter.Limit
	if limit <= 0 {
		// getGraph просит всё; потолок защищает от случайного полного скана.
		limit = 10_000
	}

	rows, err := d.Pgx().Query(ctx, graphNodeSelect+`
		 WHERE n.investigation_id = $1::uuid
		   AND ($2::text IS NULL OR n.node_type = $2)
		   AND ($3::text IS NULL
		        OR e.event_type ILIKE '%' || $3 || '%'
		        OR ent.display_name ILIKE '%' || $3 || '%'
		        OR ent.canonical_key ILIKE '%' || $3 || '%')
		 GROUP BY n.id, e.event_type, e.occurred_at, ent.display_name, ent.canonical_key
		 ORDER BY n.created_at, n.id
		 LIMIT $4
	`, investigationID, filter.NodeType, filter.Q, limit)
	if err != nil {
		return nil, fmt.Errorf("query graph nodes: %w", err)
	}
	defer rows.Close()

	var nodes []model.GraphNode
	for rows.Next() {
		node, err := scanGraphNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// Общая проекция ноды: label событий — event_type, entity — display_name/
// canonical_key; som_issue_ids сворачиваются в массив.
const graphNodeSelect = `
		SELECT n.id::text, n.investigation_id::text, n.node_type,
		       n.entity_id::text, n.event_id::text, n.origin,
		       COALESCE(array_agg(gsi.som_issue_id::text)
		                FILTER (WHERE gsi.som_issue_id IS NOT NULL), '{}') AS som_issue_ids,
		       COALESCE(e.event_type, ent.display_name, ent.canonical_key) AS label,
		       e.occurred_at
		  FROM graph_nodes n
		  LEFT JOIN graph_node_som_issues gsi ON gsi.graph_node_id = n.id
		  LEFT JOIN events e ON e.id = n.event_id
		  LEFT JOIN entities ent ON ent.id = n.entity_id
`

func graphNodeTx(ctx context.Context, tx pgx.Tx, nodeID string) (model.GraphNode, error) {
	row := tx.QueryRow(ctx, graphNodeSelect+`
		 WHERE n.id = $1::uuid
		 GROUP BY n.id, e.event_type, e.occurred_at, ent.display_name, ent.canonical_key
	`, nodeID)
	return scanGraphNode(row)
}

func scanGraphNode(row pgx.Row) (model.GraphNode, error) {
	var node model.GraphNode
	err := row.Scan(&node.ID, &node.InvestigationID, &node.NodeType,
		&node.EntityID, &node.EventID, &node.Origin, &node.SomIssueIDs,
		&node.Label, &node.OccurredAt)
	if err != nil {
		return model.GraphNode{}, fmt.Errorf("scan graph node: %w", err)
	}
	return node, nil
}

func investigationExistsTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string) (bool, error) {
	var one int
	err := tx.QueryRow(ctx, `
		SELECT 1 FROM investigations WHERE id = $1::uuid AND project_id = $2
	`, investigationID, projectID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgInvalidTextRepresentation {
			return false, nil
		}
		return false, fmt.Errorf("check investigation: %w", err)
	}
	return true, nil
}

func linkEventTx(ctx context.Context, tx pgx.Tx, projectID, investigationID, eventID string,
	attachedBy string, reason *string) (bool, error) {

	tag, err := tx.Exec(ctx, `
		INSERT INTO investigation_events (investigation_id, event_id, project_id, attached_by, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
		ON CONFLICT DO NOTHING
	`, investigationID, eventID, projectID, attachedBy, reason)
	if err != nil {
		return false, fmt.Errorf("link event %s: %w", eventID, err)
	}
	return tag.RowsAffected() > 0, nil
}

// nullableJSON превращает пустой payload в NULL: колонка raw_data nullable,
// а '' невалидный jsonb.
func nullableJSON(raw []byte) *string {
	if len(raw) == 0 {
		return nil
	}
	value := string(raw)
	return &value
}
