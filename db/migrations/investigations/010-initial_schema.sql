-- Migration: 90ed76030198

BEGIN;

SET LOCAL search_path = :"DB_INV_SCHEMA_NAME", public, pg_temp;

-- FUNCTIONS

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- REFERENCE DATA

CREATE TABLE IF NOT EXISTS sources (
    code VARCHAR(32) NOT NULL,

    kind VARCHAR(16) NOT NULL
        CHECK (kind IN ('siem', 'edr', 'ndr', 'sandbox', 'infra_logs')),
    title VARCHAR NOT NULL,
    is_enabled BOOLEAN DEFAULT true NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_sources PRIMARY KEY (code),
    CONSTRAINT uq_sources_title UNIQUE (title)
);

DROP TRIGGER IF EXISTS trg_sources_set_updated_at ON sources;
CREATE TRIGGER trg_sources_set_updated_at
BEFORE UPDATE ON sources
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS entity_types (
    code VARCHAR(64) NOT NULL,

    title VARCHAR NOT NULL,
    category VARCHAR(32) NOT NULL
        CHECK (category IN ('identity', 'network', 'execution', 'persistence', 'asset')),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_entity_types PRIMARY KEY (code),
    CONSTRAINT uq_entity_types_title UNIQUE (title)
);

DROP TRIGGER IF EXISTS trg_entity_types_set_updated_at ON entity_types;
CREATE TRIGGER trg_entity_types_set_updated_at
BEFORE UPDATE ON entity_types
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS relation_types (
    code VARCHAR(64) NOT NULL,

    title VARCHAR NOT NULL,
    source_kind VARCHAR(8) NOT NULL CHECK (source_kind IN ('entity', 'event')),
    target_kind VARCHAR(8) NOT NULL CHECK (target_kind IN ('entity', 'event')),
    directed BOOLEAN DEFAULT true NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_relation_types PRIMARY KEY (code),
    CONSTRAINT uq_relation_types_title UNIQUE (title)
);

DROP TRIGGER IF EXISTS trg_relation_types_set_updated_at ON relation_types;
CREATE TRIGGER trg_relation_types_set_updated_at
BEFORE UPDATE ON relation_types
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- INVESTIGATIONS TREE

CREATE TABLE IF NOT EXISTS investigations (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    parent_id UUID,

    project_id VARCHAR(12),
    workspace_id UUID,

    title VARCHAR NOT NULL,
    description VARCHAR,
    status VARCHAR(8) DEFAULT 'open' NOT NULL
        CHECK (status IN ('open', 'closed')),
    severity VARCHAR(8)
        CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    verdict VARCHAR(16)
        CHECK (verdict IN ('incident', 'false_positive', 'not_affected',
                           'inconclusive', 'confirmed', 'rejected')),
    verdict_reason VARCHAR,
    confidence REAL CHECK (confidence >= 0 AND confidence <= 1),
    origin VARCHAR(8) DEFAULT 'analyst' NOT NULL
        CHECK (origin IN ('analyst', 'rule', 'agent')),
    origin_ref VARCHAR,
    version INTEGER DEFAULT 1 NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    closed_at TIMESTAMP WITH TIME ZONE,

    CONSTRAINT pk_investigations PRIMARY KEY (id),
    CONSTRAINT uq_investigations_id_project UNIQUE (id, project_id),
    CONSTRAINT fk_investigations_parent_id_investigations FOREIGN KEY (parent_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT ck_investigations_closed_verdict
        CHECK (status <> 'closed' OR verdict IS NOT NULL),
    CONSTRAINT ck_investigations_rejected_reason
        CHECK (verdict <> 'rejected' OR verdict_reason IS NOT NULL)
);

DROP TRIGGER IF EXISTS trg_investigations_set_updated_at ON investigations;
CREATE TRIGGER trg_investigations_set_updated_at
BEFORE UPDATE ON investigations
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_investigations_project_created_at
    ON investigations (project_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS ix_investigations_parent
    ON investigations (parent_id);

CREATE INDEX IF NOT EXISTS ix_investigations_status
    ON investigations (project_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS ix_investigations_workspace
    ON investigations (workspace_id)
    WHERE workspace_id IS NOT NULL;

-- EVIDENCE

CREATE TABLE IF NOT EXISTS events (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    source_code VARCHAR(32) NOT NULL,

    source_event_id VARCHAR NOT NULL,
    source_ref VARCHAR,
    event_type VARCHAR(64) NOT NULL,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ingested_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    normalized_data JSONB DEFAULT '{}'::jsonb NOT NULL,
    raw_data JSONB,
    dedup_key VARCHAR NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_events PRIMARY KEY (id),
    CONSTRAINT uq_events_id_project UNIQUE (id, project_id),
    CONSTRAINT uq_events_dedup UNIQUE (project_id, dedup_key),
    CONSTRAINT uq_events_source UNIQUE (project_id, source_code, source_event_id),
    CONSTRAINT fk_events_source_code_sources FOREIGN KEY (source_code)
        REFERENCES sources (code)
);

DROP TRIGGER IF EXISTS trg_events_set_updated_at ON events;
CREATE TRIGGER trg_events_set_updated_at
BEFORE UPDATE ON events
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_events_timeline
    ON events (project_id, occurred_at, id);

CREATE INDEX IF NOT EXISTS ix_events_normalized
    ON events USING gin (normalized_data jsonb_path_ops);

CREATE INDEX IF NOT EXISTS ix_events_normalized_trgm
    ON events USING gin ((normalized_data::text) gin_trgm_ops);

CREATE TABLE IF NOT EXISTS entities (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    type_code VARCHAR(64) NOT NULL,

    canonical_key VARCHAR NOT NULL,
    display_name VARCHAR,
    metadata JSONB DEFAULT '{}'::jsonb NOT NULL,
    first_seen TIMESTAMP WITH TIME ZONE,
    last_seen TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_entities PRIMARY KEY (id),
    CONSTRAINT uq_entities_id_project UNIQUE (id, project_id),
    CONSTRAINT uq_entities_scope_type_key UNIQUE (project_id, type_code, canonical_key),
    CONSTRAINT fk_entities_type_code_entity_types FOREIGN KEY (type_code)
        REFERENCES entity_types (code)
);

DROP TRIGGER IF EXISTS trg_entities_set_updated_at ON entities;
CREATE TRIGGER trg_entities_set_updated_at
BEFORE UPDATE ON entities
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS event_entity_relations (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    event_id UUID NOT NULL,
    entity_id UUID NOT NULL,

    relation_code VARCHAR(64) NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_event_entity_relations PRIMARY KEY (id),
    CONSTRAINT uq_event_entity_relations UNIQUE (event_id, entity_id, relation_code),
    CONSTRAINT fk_eer_event_id_events FOREIGN KEY (event_id, project_id)
        REFERENCES events (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_eer_entity_id_entities FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_eer_relation_code_relation_types FOREIGN KEY (relation_code)
        REFERENCES relation_types (code)
);

DROP TRIGGER IF EXISTS trg_event_entity_relations_set_updated_at ON event_entity_relations;
CREATE TRIGGER trg_event_entity_relations_set_updated_at
BEFORE UPDATE ON event_entity_relations
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_eer_entity ON event_entity_relations (entity_id);
CREATE INDEX IF NOT EXISTS ix_eer_event ON event_entity_relations (event_id);

-- INVESTIGATION COMPONENTS

CREATE TABLE IF NOT EXISTS investigation_events (
    investigation_id UUID NOT NULL,
    event_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,

    attached_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    attached_by VARCHAR(16) DEFAULT 'analyst' NOT NULL
        CHECK (attached_by IN ('analyst', 'agent', 'system')),
    reason VARCHAR,

    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_events PRIMARY KEY (investigation_id, event_id),
    CONSTRAINT uq_inv_events_project UNIQUE (investigation_id, event_id, project_id),
    CONSTRAINT fk_inv_events_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_inv_events_event FOREIGN KEY (event_id, project_id)
        REFERENCES events (id, project_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_investigation_events_set_updated_at ON investigation_events;
CREATE TRIGGER trg_investigation_events_set_updated_at
BEFORE UPDATE ON investigation_events
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_inv_events_event ON investigation_events (event_id);

CREATE TABLE IF NOT EXISTS investigation_entities (
    investigation_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,

    added_via VARCHAR(16) DEFAULT 'event' NOT NULL
        CHECK (added_via IN ('event', 'ioc', 'agent', 'analyst')),
    added_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_entities PRIMARY KEY (investigation_id, entity_id),
    CONSTRAINT uq_inv_entities_project UNIQUE (investigation_id, entity_id, project_id),
    CONSTRAINT fk_inv_entities_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_inv_entities_entity FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_investigation_entities_set_updated_at ON investigation_entities;
CREATE TRIGGER trg_investigation_entities_set_updated_at
BEFORE UPDATE ON investigation_entities
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_inv_entities_entity ON investigation_entities (entity_id);

-- GRAPH

CREATE TABLE IF NOT EXISTS graph_nodes (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    investigation_id UUID NOT NULL,

    node_type VARCHAR(8) NOT NULL CHECK (node_type IN ('entity', 'event')),
    entity_id UUID,
    event_id UUID,
    origin VARCHAR(8) NOT NULL CHECK (origin IN ('analyst', 'rule', 'agent')),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_graph_nodes PRIMARY KEY (id),
    CONSTRAINT uq_graph_nodes_id_investigation UNIQUE (id, investigation_id),
    CONSTRAINT ck_graph_nodes_target CHECK (
        (node_type = 'entity' AND entity_id IS NOT NULL AND event_id IS NULL) OR
        (node_type = 'event'  AND event_id  IS NOT NULL AND entity_id IS NULL)
    ),
    CONSTRAINT fk_graph_nodes_investigation_id_investigations FOREIGN KEY (investigation_id)
        REFERENCES investigations (id) ON DELETE CASCADE,
    CONSTRAINT fk_graph_nodes_entity FOREIGN KEY (investigation_id, entity_id)
        REFERENCES investigation_entities (investigation_id, entity_id) ON DELETE CASCADE,
    CONSTRAINT fk_graph_nodes_event FOREIGN KEY (investigation_id, event_id)
        REFERENCES investigation_events (investigation_id, event_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_graph_nodes_set_updated_at ON graph_nodes;
CREATE TRIGGER trg_graph_nodes_set_updated_at
BEFORE UPDATE ON graph_nodes
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX IF NOT EXISTS uq_graph_nodes_entity
    ON graph_nodes (investigation_id, entity_id) WHERE entity_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_graph_nodes_event
    ON graph_nodes (investigation_id, event_id) WHERE event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS graph_node_som_issues (
    graph_node_id UUID NOT NULL,
    som_issue_id UUID NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_graph_node_som_issues PRIMARY KEY (graph_node_id, som_issue_id),
    CONSTRAINT fk_graph_node_som_issues_node FOREIGN KEY (graph_node_id)
        REFERENCES graph_nodes (id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_graph_node_som_issues_set_updated_at ON graph_node_som_issues;
CREATE TRIGGER trg_graph_node_som_issues_set_updated_at
BEFORE UPDATE ON graph_node_som_issues
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_graph_node_som_issues_issue
    ON graph_node_som_issues (som_issue_id);

CREATE TABLE IF NOT EXISTS edges (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    investigation_id UUID NOT NULL,

    source_node_id UUID NOT NULL,
    target_node_id UUID NOT NULL,
    relation_code VARCHAR(64) NOT NULL,
    status VARCHAR(16) DEFAULT 'proposed' NOT NULL
        CHECK (status IN ('proposed', 'confirmed', 'rejected')),
    reject_reason VARCHAR,
    confidence REAL CHECK (confidence >= 0 AND confidence <= 1),
    why VARCHAR,
    origin VARCHAR(8) NOT NULL CHECK (origin IN ('analyst', 'rule', 'agent')),
    origin_ref VARCHAR,
    metadata JSONB DEFAULT '{}'::jsonb NOT NULL,
    version INTEGER DEFAULT 1 NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_edges PRIMARY KEY (id),
    CONSTRAINT uq_edges_id_investigation UNIQUE (id, investigation_id),
    CONSTRAINT uq_edges_triple UNIQUE (investigation_id, source_node_id, target_node_id, relation_code),
    CONSTRAINT ck_edges_rejected_reason
        CHECK (status <> 'rejected' OR reject_reason IS NOT NULL),
    CONSTRAINT fk_edges_investigation_id_investigations FOREIGN KEY (investigation_id)
        REFERENCES investigations (id) ON DELETE CASCADE,
    CONSTRAINT fk_edges_source_node_id_graph_nodes FOREIGN KEY (source_node_id, investigation_id)
        REFERENCES graph_nodes (id, investigation_id) ON DELETE CASCADE,
    CONSTRAINT fk_edges_target_node_id_graph_nodes FOREIGN KEY (target_node_id, investigation_id)
        REFERENCES graph_nodes (id, investigation_id) ON DELETE CASCADE,
    CONSTRAINT fk_edges_relation_code_relation_types FOREIGN KEY (relation_code)
        REFERENCES relation_types (code)
);

DROP TRIGGER IF EXISTS trg_edges_set_updated_at ON edges;
CREATE TRIGGER trg_edges_set_updated_at
BEFORE UPDATE ON edges
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_edges_investigation_status
    ON edges (investigation_id, status);
CREATE INDEX IF NOT EXISTS ix_edges_source ON edges (source_node_id);
CREATE INDEX IF NOT EXISTS ix_edges_target ON edges (target_node_id);

CREATE TABLE IF NOT EXISTS edge_evidence (
    edge_id UUID NOT NULL,
    event_id UUID NOT NULL,
    investigation_id UUID NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_edge_evidence PRIMARY KEY (edge_id, event_id),
    CONSTRAINT fk_edge_evidence_edge FOREIGN KEY (edge_id, investigation_id)
        REFERENCES edges (id, investigation_id) ON DELETE CASCADE,
    CONSTRAINT fk_edge_evidence_event FOREIGN KEY (investigation_id, event_id)
        REFERENCES investigation_events (investigation_id, event_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_edge_evidence_set_updated_at ON edge_evidence;
CREATE TRIGGER trg_edge_evidence_set_updated_at
BEFORE UPDATE ON edge_evidence
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_edge_evidence_event ON edge_evidence (event_id);

-- ACCESS CONTROL

CREATE TABLE IF NOT EXISTS role_bindings (
    project_id VARCHAR(12) NOT NULL,
    subject_id UUID NOT NULL,
    role VARCHAR(8) NOT NULL CHECK (role IN ('l1', 'l2', 'lead', 'admin')),

    granted_by_subject_id UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_role_bindings PRIMARY KEY (project_id, subject_id, role)
);

CREATE INDEX IF NOT EXISTS ix_role_bindings_subject ON role_bindings (subject_id);

WITH updated AS (
    UPDATE version_investigations
    SET version_num = '90ed76030198'
    RETURNING version_investigations.version_num
)
INSERT INTO version_investigations (version_num)
SELECT '90ed76030198'
WHERE NOT EXISTS (SELECT 1 FROM updated)
RETURNING version_num;

COMMIT;
