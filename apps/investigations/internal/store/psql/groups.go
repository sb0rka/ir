package psql

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/grouping"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

// Table identifiers are constants selected here, never interpolated user values.
func groupTables(family string) (groups, members, objects, column, lineage string, err error) {
	switch family {
	case "entity":
		return "entity_resolution_groups", "entity_resolution_members", "entities", "entity_id", "entity_group_lineage", nil
	case "event":
		return "event_groups", "event_group_members", "events", "event_id", "event_group_lineage", nil
	default:
		return "", "", "", "", "", store.ErrInvalidValue
	}
}

const groupTreeCTE = `WITH RECURSIVE tree AS (
 SELECT id FROM investigations WHERE project_id=$1 AND id=$2::uuid AND parent_id IS NULL AND is_deleted=false
 UNION ALL SELECT i.id FROM investigations i JOIN tree t ON i.parent_id=t.id
 WHERE i.project_id=$1 AND i.is_deleted=false
) `

func groupScopeTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string, lock bool) (model.GroupScope, error) {
	scope := model.GroupScope{ProjectID: projectID}
	if !grouping.ValidID(investigationID) {
		return scope, store.ErrRecordNotFound
	}
	err := tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS (
 SELECT id,parent_id,ARRAY[id] AS path FROM investigations WHERE id=$1::uuid AND project_id=$2 AND is_deleted=false
 UNION ALL SELECT p.id,p.parent_id,a.path||p.id FROM investigations p JOIN ancestors a ON a.parent_id=p.id
 WHERE p.project_id=$2 AND p.is_deleted=false AND NOT p.id=ANY(a.path)
) SELECT id::text FROM ancestors WHERE parent_id IS NULL`, investigationID, projectID).Scan(&scope.RootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return scope, store.ErrRecordNotFound
	}
	if err != nil {
		return scope, err
	}
	if lock {
		// ponytail: serialize mutations per tree; replace with narrower locks only after measured contention.
		err = tx.QueryRow(ctx, `SELECT id::text FROM investigations WHERE id=$1::uuid AND project_id=$2 AND parent_id IS NULL AND is_deleted=false FOR UPDATE`, scope.RootID, projectID).Scan(&scope.RootID)
		if errors.Is(err, pgx.ErrNoRows) {
			return scope, store.ErrRecordNotFound
		}
	}
	return scope, err
}

func requireGroupRootTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, lock bool) error {
	resolved, err := groupScopeTx(ctx, tx, scope.ProjectID, scope.RootID, lock)
	if err != nil {
		return err
	}
	if resolved != scope {
		return store.ErrRecordNotFound
	}
	return nil
}

func getGroupTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, family, id string) (model.Group, error) {
	g := model.Group{GroupScope: scope, Family: family, Members: []model.GroupMember{}, SuccessorIDs: []string{}}
	table, members, _, column, lineage, err := groupTables(family)
	if err != nil {
		return g, err
	}
	if !grouping.ValidID(id) {
		return g, store.ErrRecordNotFound
	}
	err = tx.QueryRow(ctx, `SELECT id::text,kind,COALESCE(type_code,''),group_key,title,state,version,created_at,updated_at FROM `+table+` WHERE project_id=$1 AND root_investigation_id=$2::uuid AND id=$3::uuid`, scope.ProjectID, scope.RootID, id).
		Scan(&g.ID, &g.Kind, &g.TypeCode, &g.Key, &g.Title, &g.State, &g.Version, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, store.ErrRecordNotFound
	}
	if err != nil {
		return g, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,`+column+`::text,role,ordinal,status,confidence,decision_reason,version,assertions FROM `+members+` WHERE project_id=$1 AND root_investigation_id=$2::uuid AND group_id=$3::uuid ORDER BY id`, scope.ProjectID, scope.RootID, id)
	if err != nil {
		return g, err
	}
	for rows.Next() {
		var m model.GroupMember
		var raw []byte
		if err = rows.Scan(&m.ID, &m.ObjectID, &m.Role, &m.Ordinal, &m.Status, &m.Confidence, &m.Reason, &m.Version, &raw); err == nil {
			err = json.Unmarshal(raw, &m.Assertions)
		}
		if err != nil {
			rows.Close()
			return g, err
		}
		g.Members = append(g.Members, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return g, err
	}
	rows, err = tx.Query(ctx, `SELECT successor_id::text FROM `+lineage+` WHERE project_id=$1 AND root_investigation_id=$2::uuid AND predecessor_id=$3::uuid ORDER BY successor_id`, scope.ProjectID, scope.RootID, id)
	if err != nil {
		return g, err
	}
	defer rows.Close()
	for rows.Next() {
		var successor string
		if err = rows.Scan(&successor); err != nil {
			return g, err
		}
		g.SuccessorIDs = append(g.SuccessorIDs, successor)
	}
	return g, rows.Err()
}

func (d *DB) GetGroup(ctx context.Context, scope model.GroupScope, family, id string) (model.Group, error) {
	tx, err := d.Pgx().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return model.Group{}, err
	}
	defer tx.Rollback(ctx)
	if err = requireGroupRootTx(ctx, tx, scope, false); err != nil {
		return model.Group{}, err
	}
	return getGroupTx(ctx, tx, scope, family, id)
}

func groupDomainError(err error) error {
	switch {
	case errors.Is(err, grouping.ErrInvalid):
		return store.ErrInvalidValue
	case errors.Is(err, grouping.ErrConflict):
		return store.ErrConflict
	case errors.Is(err, grouping.ErrNotFound):
		return store.ErrRecordNotFound
	default:
		return mapConstraint(err)
	}
}

func saveGroupTx(ctx context.Context, tx pgx.Tx, g *model.Group) error {
	if err := grouping.Validate(*g); err != nil {
		return groupDomainError(err)
	}
	table, members, _, column, _, err := groupTables(g.Family)
	if err != nil {
		return err
	}
	var typ any
	if g.Family == "entity" {
		typ = g.TypeCode
	}
	err = tx.QueryRow(ctx, `INSERT INTO `+table+` (id,project_id,root_investigation_id,kind,type_code,group_key,title,state,version)
 VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9)
 ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,state=EXCLUDED.state,version=EXCLUDED.version,updated_at=now()
 WHERE `+table+`.project_id=EXCLUDED.project_id AND `+table+`.root_investigation_id=EXCLUDED.root_investigation_id
 RETURNING created_at,updated_at`, g.ID, g.ProjectID, g.RootID, g.Kind, typ, g.Key, g.Title, g.State, g.Version).Scan(&g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return groupDomainError(err)
	}
	for _, m := range g.Members {
		raw, err := json.Marshal(m.Assertions)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO `+members+` (id,group_id,project_id,root_investigation_id,`+column+`,role,ordinal,status,confidence,decision_reason,version,assertions)
 VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11,$12::jsonb)
 ON CONFLICT(id) DO UPDATE SET role=EXCLUDED.role,ordinal=EXCLUDED.ordinal,status=EXCLUDED.status,confidence=EXCLUDED.confidence,
 decision_reason=EXCLUDED.decision_reason,version=EXCLUDED.version,assertions=EXCLUDED.assertions
 WHERE `+members+`.group_id=EXCLUDED.group_id AND `+members+`.project_id=EXCLUDED.project_id AND `+members+`.root_investigation_id=EXCLUDED.root_investigation_id`,
			m.ID, g.ID, g.ProjectID, g.RootID, m.ObjectID, m.Role, m.Ordinal, m.Status, m.Confidence, m.Reason, m.Version, string(raw))
		if err != nil {
			return groupDomainError(err)
		}
	}
	return nil
}

func validateGroupEvidenceTx(ctx context.Context, tx pgx.Tx, g model.Group) error {
	_, _, objects, column, _, err := groupTables(g.Family)
	if err != nil {
		return err
	}
	attachment := "investigation_" + objects
	strongSubjects := map[string]string{}
	for _, m := range g.Members {
		if m.Status == "rejected" {
			continue
		}
		eligible := false
		for _, a := range m.Assertions {
			var ok bool
			err = tx.QueryRow(ctx, groupTreeCTE+`SELECT EXISTS(SELECT 1 FROM `+attachment+` a JOIN tree t ON t.id=a.investigation_id
 WHERE a.project_id=$1 AND a.`+column+`=$3::uuid AND a.investigation_id=$4::uuid)
 AND NOT EXISTS(SELECT 1 FROM unnest($5::uuid[]) e(id) WHERE NOT EXISTS (
 SELECT 1 FROM investigation_events ie WHERE ie.project_id=$1 AND ie.investigation_id=$4::uuid AND ie.event_id=e.id))`,
				g.ProjectID, g.RootID, m.ObjectID, a.InvestigationID, a.EvidenceEventIDs).Scan(&ok)
			if err != nil {
				return err
			}
			eligible = eligible || ok
		}
		if !eligible {
			return store.ErrUnknownReference
		}
		if g.Family == "entity" && m.Role == "identifier" && m.Status == "confirmed" {
			var typ string
			if err = tx.QueryRow(ctx, `SELECT type_code FROM entities WHERE project_id=$1 AND id=$2::uuid`, g.ProjectID, m.ObjectID).Scan(&typ); err != nil {
				return groupDomainError(err)
			}
			if typ == "ip" && slices.ContainsFunc(m.Assertions, func(a model.GroupAssertion) bool { return a.ValidFrom == nil || a.ValidTo == nil }) {
				return store.ErrInvalidValue
			}
		}
		if g.Family == "entity" && m.Role == "subject" {
			var typ, method, instance string
			err = tx.QueryRow(ctx, `SELECT type_code,COALESCE(metadata->>'identity_method',''),COALESCE(metadata->>'source_instance','') FROM entities WHERE project_id=$1 AND id=$2::uuid`, g.ProjectID, m.ObjectID).Scan(&typ, &method, &instance)
			if err != nil {
				return groupDomainError(err)
			}
			if typ != g.TypeCode {
				return store.ErrInvalidValue
			}
			if m.Status == "confirmed" && method == "pt-nad-host-id" && instance != "" {
				if prior := strongSubjects[instance]; prior != "" && prior != m.ObjectID {
					return store.ErrConflict
				}
				strongSubjects[instance] = m.ObjectID
			}
		}
	}
	return nil
}

func checkGroupUniquenessTx(ctx context.Context, tx pgx.Tx, g model.Group) error {
	if g.State != "active" {
		return nil
	}
	table, members, _, column, _, err := groupTables(g.Family)
	if err != nil {
		return err
	}
	for _, m := range g.Members {
		if m.Status != "confirmed" || !(g.Family == "entity" && m.Role == "subject" || g.Kind == "same_event") {
			continue
		}
		var conflict bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+members+` m JOIN `+table+` g ON g.id=m.group_id
 WHERE m.project_id=$1 AND m.root_investigation_id=$2::uuid AND m.`+column+`=$3::uuid
 AND m.group_id<>$4::uuid AND m.status='confirmed' AND g.state='active'
 AND (($5='entity' AND m.role='subject') OR ($5='event' AND g.kind='same_event')))`, g.ProjectID, g.RootID, m.ObjectID, g.ID, g.Family).Scan(&conflict)
		if err != nil {
			return err
		}
		if conflict {
			return store.ErrConflict
		}
	}
	return nil
}

func operationHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func operationRetryTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, key, hash string) ([]model.Group, bool, error) {
	var prior string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT payload_hash,after_state FROM group_operations WHERE project_id=$1 AND root_investigation_id=$2::uuid AND operation_key=$3`, scope.ProjectID, scope.RootID, key).Scan(&prior, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if prior != hash {
		return nil, true, store.ErrConflict
	}
	var groups []model.Group
	err = json.Unmarshal(raw, &groups)
	return groups, true, err
}

func recordGroupOperationTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, key, hash, kind, actor, reason string, before, after []model.Group) error {
	if before == nil {
		before = []model.Group{}
	}
	if after == nil {
		after = []model.Group{}
	}
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	id := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO group_operations(id,project_id,root_investigation_id,operation_key,payload_hash,kind,actor,reason,before_state,after_state)
 VALUES($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb)`, id, scope.ProjectID, scope.RootID, key, hash, kind, actor, reason, string(b), string(a))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, g := range append(slices.Clone(before), after...) {
		if g.GroupScope != scope {
			return store.ErrRecordNotFound
		}
		if seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		var entity, event any
		if g.Family == "entity" {
			entity = g.ID
		} else {
			event = g.ID
		}
		_, err = tx.Exec(ctx, `INSERT INTO group_operation_links(operation_id,project_id,root_investigation_id,entity_group_id,event_group_id) VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5::uuid)`, id, scope.ProjectID, scope.RootID, entity, event)
		if err != nil {
			return err
		}
	}
	return nil
}

func supersedeGroupTx(ctx context.Context, tx pgx.Tx, g *model.Group, successors []string) error {
	_, _, _, _, lineage, err := groupTables(g.Family)
	if err != nil {
		return err
	}
	g.State = "superseded"
	g.Version++
	g.SuccessorIDs = slices.Clone(successors)
	if err = saveGroupTx(ctx, tx, g); err != nil {
		return err
	}
	for _, id := range successors {
		_, err = tx.Exec(ctx, `INSERT INTO `+lineage+`(project_id,root_investigation_id,predecessor_id,successor_id) VALUES($1,$2::uuid,$3::uuid,$4::uuid) ON CONFLICT DO NOTHING`, g.ProjectID, g.RootID, g.ID, id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) MutateGroup(ctx context.Context, r model.GroupMutation) ([]model.Group, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = requireGroupRootTx(ctx, tx, r.GroupScope, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.Actor) == "" {
		return nil, store.ErrInvalidValue
	}
	if (r.Review == nil && r.Merge == nil && r.Split == nil) || (r.Review != nil && r.Merge != nil) || (r.Review != nil && r.Split != nil) || (r.Merge != nil && r.Split != nil) {
		return nil, store.ErrInvalidValue
	}
	var opID, reason, kind string
	switch {
	case r.Review != nil:
		opID, reason, kind = r.Review.OperationID, r.Review.Reason, "review"
	case r.Merge != nil:
		opID, reason, kind = r.Merge.OperationID, r.Merge.Reason, "merge"
	case r.Split != nil:
		opID, reason, kind = r.Split.OperationID, r.Split.Reason, "split"
	}
	if !grouping.ValidID(opID) {
		return nil, store.ErrInvalidValue
	}
	// Validate target before idempotency lookup, so a foreign target cannot expose a prior result.
	g, err := getGroupTx(ctx, tx, r.GroupScope, r.Family, r.GroupID)
	if err != nil {
		return nil, err
	}
	hash, err := operationHash(r)
	if err != nil {
		return nil, err
	}
	if prior, found, err := operationRetryTx(ctx, tx, r.GroupScope, "operation:"+opID, hash); found || err != nil {
		return prior, err
	}
	before := []model.Group{grouping.Clone(g)}
	var after []model.Group
	switch {
	case r.Review != nil:
		updated, e := grouping.Review(g, *r.Review)
		if e != nil {
			return nil, groupDomainError(e)
		}
		if err = validateGroupEvidenceTx(ctx, tx, updated); err != nil {
			return nil, err
		}
		if err = checkGroupUniquenessTx(ctx, tx, updated); err != nil {
			return nil, err
		}
		if err = saveGroupTx(ctx, tx, &updated); err != nil {
			return nil, err
		}
		after = []model.Group{updated}
	case r.Merge != nil:
		sources := make([]model.Group, 0, len(r.Merge.Sources))
		seen := map[string]bool{g.ID: true}
		if len(r.Merge.Sources) > 100 {
			return nil, store.ErrInvalidValue
		}
		for _, source := range r.Merge.Sources {
			if seen[source.ID] {
				return nil, store.ErrInvalidValue
			}
			seen[source.ID] = true
			s, e := getGroupTx(ctx, tx, r.GroupScope, r.Family, source.ID)
			if e != nil {
				return nil, e
			}
			if s.Version != source.Version {
				return nil, store.ErrConflict
			}
			sources = append(sources, s)
			before = append(before, grouping.Clone(s))
		}
		updated, e := grouping.Merge(g, sources, *r.Merge)
		if e != nil {
			return nil, groupDomainError(e)
		}
		if err = validateGroupEvidenceTx(ctx, tx, updated); err != nil {
			return nil, err
		}
		// Sources must be superseded before checking the survivor's exclusive memberships.
		for i := range sources {
			if err = supersedeGroupTx(ctx, tx, &sources[i], []string{g.ID}); err != nil {
				return nil, err
			}
		}
		if err = checkGroupUniquenessTx(ctx, tx, updated); err != nil {
			return nil, err
		}
		if err = saveGroupTx(ctx, tx, &updated); err != nil {
			return nil, err
		}
		after = append([]model.Group{updated}, sources...)
	case r.Split != nil:
		parts, e := grouping.Split(g, *r.Split)
		if e != nil {
			return nil, groupDomainError(e)
		}
		// Temporarily supersede under the root lock; any invalid partition rolls back all writes.
		g.State = "superseded"
		g.Version++
		if err = saveGroupTx(ctx, tx, &g); err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(parts))
		for i := range parts {
			if err = validateGroupEvidenceTx(ctx, tx, parts[i]); err != nil {
				return nil, err
			}
			if err = checkGroupUniquenessTx(ctx, tx, parts[i]); err != nil {
				return nil, err
			}
			if err = saveGroupTx(ctx, tx, &parts[i]); err != nil {
				return nil, err
			}
			ids = append(ids, parts[i].ID)
		}
		g.Version--
		if err = supersedeGroupTx(ctx, tx, &g, ids); err != nil {
			return nil, err
		}
		after = append([]model.Group{g}, parts...)
	}
	if err = recordGroupOperationTx(ctx, tx, r.GroupScope, "operation:"+opID, hash, kind, r.Actor, reason, before, after); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return after, nil
}

type groupHistoryCursor struct {
	Project, Root, Family, Group, ID string
	At                               time.Time
}

func (d *DB) GroupHistory(ctx context.Context, scope model.GroupScope, family, id string, cursor *string, limit int) (model.GroupHistory, error) {
	out := model.GroupHistory{Operations: []model.GroupOperation{}}
	if limit < 1 || limit > 200 {
		return out, store.ErrInvalidValue
	}
	tx, err := d.Pgx().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = requireGroupRootTx(ctx, tx, scope, false); err != nil {
		return out, err
	}
	if _, err = getGroupTx(ctx, tx, scope, family, id); err != nil {
		return out, err
	}
	var at, previous any
	if cursor != nil && *cursor != "" {
		var c groupHistoryCursor
		raw, e := base64.RawURLEncoding.DecodeString(*cursor)
		if e != nil {
			return out, store.ErrInvalidValue
		}
		if e = json.Unmarshal(raw, &c); e != nil || c.Project != scope.ProjectID || c.Root != scope.RootID || c.Family != family || c.Group != id || !grouping.ValidID(c.ID) || c.At.IsZero() {
			return out, store.ErrInvalidValue
		}
		at, previous = c.At, c.ID
	}
	column := "entity_group_id"
	if family == "event" {
		column = "event_group_id"
	}
	rows, err := tx.Query(ctx, `SELECT o.id::text,o.operation_key,o.kind,o.actor,o.reason,o.before_state,o.after_state,o.created_at
 FROM group_operations o JOIN group_operation_links l ON l.operation_id=o.id AND l.project_id=o.project_id AND l.root_investigation_id=o.root_investigation_id
 WHERE o.project_id=$1 AND o.root_investigation_id=$2::uuid AND l.`+column+`=$3::uuid
 AND ($4::timestamptz IS NULL OR (o.created_at,o.id)<($4,$5::uuid)) ORDER BY o.created_at DESC,o.id DESC LIMIT $6`, scope.ProjectID, scope.RootID, id, at, previous, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var op model.GroupOperation
		op.GroupScope = scope
		var b, a []byte
		if err = rows.Scan(&op.ID, &op.OperationID, &op.Kind, &op.Actor, &op.Reason, &b, &a, &op.CreatedAt); err != nil {
			return out, err
		}
		if err = json.Unmarshal(b, &op.Before); err != nil {
			return out, err
		}
		if err = json.Unmarshal(a, &op.After); err != nil {
			return out, err
		}
		out.Operations = append(out.Operations, op)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Operations) > limit {
		last := out.Operations[limit-1]
		raw, _ := json.Marshal(groupHistoryCursor{scope.ProjectID, scope.RootID, family, id, last.ID, last.CreatedAt})
		value := base64.RawURLEncoding.EncodeToString(raw)
		out.NextCursor = &value
		out.Operations = out.Operations[:limit]
	}
	return out, nil
}

func listTreeGroupsTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, family string, objectIDs []string) ([]model.Group, error) {
	table, members, _, column, _, err := groupTables(family)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT g.id::text FROM `+table+` g WHERE g.project_id=$1 AND g.root_investigation_id=$2::uuid AND g.state='active'
 AND EXISTS(SELECT 1 FROM `+members+` m WHERE m.group_id=g.id AND m.project_id=$1 AND m.root_investigation_id=$2::uuid AND m.`+column+`=ANY($3::uuid[])) ORDER BY g.id`, scope.ProjectID, scope.RootID, objectIDs)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	groups := make([]model.Group, 0, len(ids))
	for _, id := range ids {
		g, e := getGroupTx(ctx, tx, scope, family, id)
		if e != nil {
			return nil, e
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func groupResult(g model.Group) model.GroupImportResult {
	out := model.GroupImportResult{GroupID: g.ID, Family: g.Family, RootID: g.RootID, MemberIDs: []string{}}
	for _, m := range g.Members {
		out.MemberIDs = append(out.MemberIDs, m.ID)
	}
	slices.Sort(out.MemberIDs)
	return out
}
