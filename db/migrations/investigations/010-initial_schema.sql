-- Migration: 81cde9c33b72

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
        CHECK (kind IN ('siem', 'edr', 'ndr', 'sandbox', 'infra', 'other')),
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
        CHECK (category IN ('identity', 'network', 'execution', 'persistence', 'asset', 'other')),

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

    project_id VARCHAR(12) NOT NULL,

    title VARCHAR NOT NULL,
    description VARCHAR,
    status VARCHAR(8) DEFAULT 'open' NOT NULL
        CHECK (status IN ('open', 'closed')),
    severity VARCHAR(8)
        CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    verdict VARCHAR(16)
        CHECK (verdict IN ('incident', 'false_positive', 'not_affected',
                           'inconclusive')),
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
        CHECK (status <> 'closed' OR verdict IS NOT NULL)
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

-- A hypothesis is a named projection of an investigation's common graph.
-- Its memberships never own or copy the underlying graph objects.
CREATE TABLE IF NOT EXISTS hypotheses (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    investigation_id UUID NOT NULL,

    statement VARCHAR(255) NOT NULL,
    description VARCHAR,
    status VARCHAR(8) DEFAULT 'proposed' NOT NULL
        CHECK (status IN ('proposed', 'active', 'resolved')),
    reason VARCHAR,
    origin VARCHAR(8) DEFAULT 'analyst' NOT NULL
        CHECK (origin IN ('analyst', 'rule', 'agent')),
    version INTEGER DEFAULT 1 NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    resolved_at TIMESTAMP WITH TIME ZONE,

    CONSTRAINT pk_hypotheses PRIMARY KEY (id),
    CONSTRAINT uq_hypotheses_id_investigation UNIQUE (id, investigation_id),
    CONSTRAINT ck_hypotheses_statement CHECK (statement ~ '[^[:space:]]'),
    CONSTRAINT ck_hypotheses_version CHECK (version >= 1),
    CONSTRAINT ck_hypotheses_resolution CHECK (
        (status = 'resolved' AND reason IS NOT NULL AND reason ~ '[^[:space:]]' AND resolved_at IS NOT NULL) OR
        (status IN ('proposed', 'active') AND reason IS NULL AND resolved_at IS NULL)
    ),
    CONSTRAINT fk_hypotheses_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_hypotheses_set_updated_at ON hypotheses;
CREATE TRIGGER trg_hypotheses_set_updated_at
BEFORE UPDATE ON hypotheses
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_hypotheses_project_investigation_created
    ON hypotheses (project_id, investigation_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS ix_hypotheses_project_investigation_status
    ON hypotheses (project_id, investigation_id, status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS investigation_som_workspaces (
    investigation_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    workspace_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_som_workspaces PRIMARY KEY (investigation_id, workspace_id),
    CONSTRAINT fk_investigation_som_workspaces_investigation
        FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_investigation_som_workspaces_workspace
    ON investigation_som_workspaces (workspace_id);

-- EVIDENCE

CREATE TABLE IF NOT EXISTS events (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    source_code VARCHAR(32) NOT NULL,

    source_event_id VARCHAR NOT NULL,
    source_ref VARCHAR,
    title VARCHAR NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ingested_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    normalized_data JSONB DEFAULT '{}'::jsonb NOT NULL,
    provenance JSONB DEFAULT '{}'::jsonb NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_events PRIMARY KEY (id),
    CONSTRAINT uq_events_id_project UNIQUE (id, project_id),
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

-- Coarse source objects are persisted independently from their investigation
-- memberships. Their time range is required for replay, but is deliberately
-- excluded from source identity.
CREATE TABLE IF NOT EXISTS findings (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    source_code VARCHAR(32) NOT NULL,
    source_instance VARCHAR NOT NULL DEFAULT '',
    record_type VARCHAR(32) NOT NULL
        CHECK (record_type IN ('siem_incident', 'siem_correlation', 'nad_attack')),
    external_id VARCHAR NOT NULL,
    time_from TIMESTAMP WITH TIME ZONE NOT NULL,
    time_to TIMESTAMP WITH TIME ZONE NOT NULL,

    kind VARCHAR(32) NOT NULL
        CHECK (kind IN ('siem_incident', 'siem_correlation', 'nad_attack')),
    title VARCHAR NOT NULL,
    description VARCHAR,
    severity VARCHAR(8) NOT NULL
        CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical', 'unknown')),
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR,
    source_ref VARCHAR,
    fetched_at TIMESTAMP WITH TIME ZONE NOT NULL,
    normalized_snapshot JSONB DEFAULT '{}'::jsonb NOT NULL,
    provenance JSONB DEFAULT '{}'::jsonb NOT NULL,
    context_status VARCHAR(8) NOT NULL
        CHECK (context_status IN ('complete', 'partial')),
    context_errors JSONB DEFAULT '[]'::jsonb NOT NULL
        CHECK (jsonb_typeof(context_errors) = 'array'),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_findings PRIMARY KEY (id),
    CONSTRAINT uq_findings_id_project UNIQUE (id, project_id),
    CONSTRAINT uq_findings_source UNIQUE
        (project_id, source_code, source_instance, record_type, external_id),
    CONSTRAINT ck_findings_time_range CHECK (time_from < time_to),
    CONSTRAINT ck_findings_kind_record_type CHECK (kind = record_type),
    CONSTRAINT fk_findings_source FOREIGN KEY (source_code) REFERENCES sources (code)
);

DROP TRIGGER IF EXISTS trg_findings_set_updated_at ON findings;
CREATE TRIGGER trg_findings_set_updated_at
BEFORE UPDATE ON findings
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_findings_project_occurred
    ON findings (project_id, occurred_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS network_sessions (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    source_code VARCHAR(32) NOT NULL,
    source_instance VARCHAR NOT NULL DEFAULT '',
    record_type VARCHAR(32) NOT NULL CHECK (record_type = 'nad_session'),
    external_id VARCHAR NOT NULL,
    time_from TIMESTAMP WITH TIME ZONE NOT NULL,
    time_to TIMESTAMP WITH TIME ZONE NOT NULL,

    title VARCHAR NOT NULL,
    severity VARCHAR(8) NOT NULL
        CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical', 'unknown')),
    started_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ended_at TIMESTAMP WITH TIME ZONE,
    source_ref VARCHAR,
    fetched_at TIMESTAMP WITH TIME ZONE NOT NULL,
    normalized_snapshot JSONB DEFAULT '{}'::jsonb NOT NULL,
    provenance JSONB DEFAULT '{}'::jsonb NOT NULL,
    context_status VARCHAR(8) NOT NULL
        CHECK (context_status IN ('complete', 'partial')),
    context_errors JSONB DEFAULT '[]'::jsonb NOT NULL
        CHECK (jsonb_typeof(context_errors) = 'array'),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_network_sessions PRIMARY KEY (id),
    CONSTRAINT uq_network_sessions_id_project UNIQUE (id, project_id),
    CONSTRAINT uq_network_sessions_source UNIQUE
        (project_id, source_code, source_instance, record_type, external_id),
    CONSTRAINT ck_network_sessions_time_range CHECK (time_from < time_to),
    CONSTRAINT ck_network_sessions_times CHECK (ended_at IS NULL OR started_at <= ended_at),
    CONSTRAINT fk_network_sessions_source FOREIGN KEY (source_code) REFERENCES sources (code)
);

DROP TRIGGER IF EXISTS trg_network_sessions_set_updated_at ON network_sessions;
CREATE TRIGGER trg_network_sessions_set_updated_at
BEFORE UPDATE ON network_sessions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_network_sessions_project_started
    ON network_sessions (project_id, started_at DESC, id DESC);

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

CREATE TABLE IF NOT EXISTS entity_sources (
    entity_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    source_code VARCHAR(32) NOT NULL,
    source_entity_id VARCHAR NOT NULL,
    source_ref VARCHAR,
    fetched_at TIMESTAMP WITH TIME ZONE NOT NULL,
    provenance JSONB DEFAULT '{}'::jsonb NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_entity_sources PRIMARY KEY (entity_id, source_code, source_entity_id),
    CONSTRAINT uq_entity_sources_external UNIQUE (project_id, source_code, source_entity_id),
    CONSTRAINT fk_entity_sources_entity FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_sources_source FOREIGN KEY (source_code)
        REFERENCES sources (code)
);

DROP TRIGGER IF EXISTS trg_entity_sources_set_updated_at ON entity_sources;
CREATE TRIGGER trg_entity_sources_set_updated_at
BEFORE UPDATE ON entity_sources
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_entity_sources_entity
    ON entity_sources (entity_id);

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

CREATE TABLE IF NOT EXISTS entity_relations (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    source_code VARCHAR(32) NOT NULL,
    source_relation_id VARCHAR NOT NULL,
    source_ref VARCHAR,
    source_entity_id UUID NOT NULL,
    target_entity_id UUID NOT NULL,
    relation_code VARCHAR(64) NOT NULL,
    occurred_at TIMESTAMP WITH TIME ZONE,
    provenance JSONB DEFAULT '{}'::jsonb NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_entity_relations PRIMARY KEY (id),
    CONSTRAINT uq_entity_relations_source UNIQUE (project_id, source_code, source_relation_id),
    CONSTRAINT fk_entity_relations_source_code FOREIGN KEY (source_code)
        REFERENCES sources (code),
    CONSTRAINT fk_entity_relations_source FOREIGN KEY (source_entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_relations_target FOREIGN KEY (target_entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_relations_type FOREIGN KEY (relation_code)
        REFERENCES relation_types (code)
);

DROP TRIGGER IF EXISTS trg_entity_relations_set_updated_at ON entity_relations;
CREATE TRIGGER trg_entity_relations_set_updated_at
BEFORE UPDATE ON entity_relations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Source-owned nesting stays first class without adding finding/session graph
-- node kinds. Granular facts below are still projected into the existing graph.
CREATE TABLE IF NOT EXISTS finding_relations (
    project_id VARCHAR(12) NOT NULL,
    source_finding_id UUID NOT NULL,
    target_finding_id UUID NOT NULL,
    relation_code VARCHAR(64) NOT NULL DEFAULT 'contains',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_finding_relations PRIMARY KEY
        (source_finding_id, target_finding_id, relation_code),
    CONSTRAINT ck_finding_relations_not_self CHECK (source_finding_id <> target_finding_id),
    CONSTRAINT fk_finding_relations_source FOREIGN KEY (source_finding_id, project_id)
        REFERENCES findings (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_finding_relations_target FOREIGN KEY (target_finding_id, project_id)
        REFERENCES findings (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_finding_relations_target
    ON finding_relations (target_finding_id);

CREATE TABLE IF NOT EXISTS finding_sessions (
    project_id VARCHAR(12) NOT NULL,
    finding_id UUID NOT NULL,
    session_id UUID NOT NULL,
    relation_code VARCHAR(64) NOT NULL DEFAULT 'related_session',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_finding_sessions PRIMARY KEY (finding_id, session_id, relation_code),
    CONSTRAINT fk_finding_sessions_finding FOREIGN KEY (finding_id, project_id)
        REFERENCES findings (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_finding_sessions_session FOREIGN KEY (session_id, project_id)
        REFERENCES network_sessions (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_finding_sessions_session ON finding_sessions (session_id);

CREATE TABLE IF NOT EXISTS finding_events (
    project_id VARCHAR(12) NOT NULL,
    finding_id UUID NOT NULL,
    event_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_finding_events PRIMARY KEY (finding_id, event_id),
    CONSTRAINT fk_finding_events_finding FOREIGN KEY (finding_id, project_id)
        REFERENCES findings (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_finding_events_event FOREIGN KEY (event_id, project_id)
        REFERENCES events (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_finding_events_event ON finding_events (event_id);

CREATE TABLE IF NOT EXISTS finding_entities (
    project_id VARCHAR(12) NOT NULL,
    finding_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_finding_entities PRIMARY KEY (finding_id, entity_id),
    CONSTRAINT fk_finding_entities_finding FOREIGN KEY (finding_id, project_id)
        REFERENCES findings (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_finding_entities_entity FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_finding_entities_entity ON finding_entities (entity_id);

CREATE TABLE IF NOT EXISTS network_session_events (
    project_id VARCHAR(12) NOT NULL,
    session_id UUID NOT NULL,
    event_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_network_session_events PRIMARY KEY (session_id, event_id),
    CONSTRAINT fk_network_session_events_session FOREIGN KEY (session_id, project_id)
        REFERENCES network_sessions (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_network_session_events_event FOREIGN KEY (event_id, project_id)
        REFERENCES events (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_network_session_events_event
    ON network_session_events (event_id);

CREATE TABLE IF NOT EXISTS network_session_entities (
    project_id VARCHAR(12) NOT NULL,
    session_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_network_session_entities PRIMARY KEY (session_id, entity_id),
    CONSTRAINT fk_network_session_entities_session FOREIGN KEY (session_id, project_id)
        REFERENCES network_sessions (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_network_session_entities_entity FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_network_session_entities_entity
    ON network_session_entities (entity_id);

-- INVESTIGATION COMPONENTS

CREATE TABLE IF NOT EXISTS investigation_findings (
    investigation_id UUID NOT NULL,
    finding_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    directly_added BOOLEAN DEFAULT false NOT NULL,
    derived BOOLEAN DEFAULT false NOT NULL,
    attached_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_findings PRIMARY KEY (investigation_id, finding_id),
    CONSTRAINT uq_inv_findings_project UNIQUE (investigation_id, finding_id, project_id),
    CONSTRAINT ck_inv_findings_origin CHECK (directly_added OR derived),
    CONSTRAINT fk_inv_findings_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_inv_findings_finding FOREIGN KEY (finding_id, project_id)
        REFERENCES findings (id, project_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_investigation_findings_set_updated_at ON investigation_findings;
CREATE TRIGGER trg_investigation_findings_set_updated_at
BEFORE UPDATE ON investigation_findings
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_inv_findings_finding ON investigation_findings (finding_id);

CREATE TABLE IF NOT EXISTS investigation_sessions (
    investigation_id UUID NOT NULL,
    session_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    directly_added BOOLEAN DEFAULT false NOT NULL,
    derived BOOLEAN DEFAULT false NOT NULL,
    attached_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_sessions PRIMARY KEY (investigation_id, session_id),
    CONSTRAINT uq_inv_sessions_project UNIQUE (investigation_id, session_id, project_id),
    CONSTRAINT ck_inv_sessions_origin CHECK (directly_added OR derived),
    CONSTRAINT fk_inv_sessions_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_inv_sessions_session FOREIGN KEY (session_id, project_id)
        REFERENCES network_sessions (id, project_id) ON DELETE CASCADE
);

DROP TRIGGER IF EXISTS trg_investigation_sessions_set_updated_at ON investigation_sessions;
CREATE TRIGGER trg_investigation_sessions_set_updated_at
BEFORE UPDATE ON investigation_sessions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS ix_inv_sessions_session ON investigation_sessions (session_id);

CREATE TABLE IF NOT EXISTS investigation_events (
    investigation_id UUID NOT NULL,
    event_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,

    attached_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    attached_by VARCHAR(16) DEFAULT 'analyst' NOT NULL
        CHECK (attached_by IN ('analyst', 'agent', 'system')),
    directly_added BOOLEAN DEFAULT true NOT NULL,
    derived BOOLEAN DEFAULT false NOT NULL,
    reason VARCHAR,

    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_events PRIMARY KEY (investigation_id, event_id),
    CONSTRAINT uq_inv_events_project UNIQUE (investigation_id, event_id, project_id),
    CONSTRAINT ck_inv_events_origin CHECK (directly_added OR derived),
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
    directly_added BOOLEAN DEFAULT true NOT NULL,
    derived BOOLEAN DEFAULT false NOT NULL,
    added_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_entities PRIMARY KEY (investigation_id, entity_id),
    CONSTRAINT uq_inv_entities_project UNIQUE (investigation_id, entity_id, project_id),
    CONSTRAINT ck_inv_entities_origin CHECK (directly_added OR derived),
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

CREATE TABLE IF NOT EXISTS hypothesis_nodes (
    hypothesis_id UUID NOT NULL,
    investigation_id UUID NOT NULL,
    node_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_hypothesis_nodes PRIMARY KEY (hypothesis_id, node_id),
    CONSTRAINT fk_hypothesis_nodes_hypothesis FOREIGN KEY (hypothesis_id, investigation_id)
        REFERENCES hypotheses (id, investigation_id) ON DELETE CASCADE,
    CONSTRAINT fk_hypothesis_nodes_node FOREIGN KEY (node_id, investigation_id)
        REFERENCES graph_nodes (id, investigation_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_hypothesis_nodes_node
    ON hypothesis_nodes (node_id, hypothesis_id);

CREATE TABLE IF NOT EXISTS hypothesis_edges (
    hypothesis_id UUID NOT NULL,
    investigation_id UUID NOT NULL,
    edge_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_hypothesis_edges PRIMARY KEY (hypothesis_id, edge_id),
    CONSTRAINT fk_hypothesis_edges_hypothesis FOREIGN KEY (hypothesis_id, investigation_id)
        REFERENCES hypotheses (id, investigation_id) ON DELETE CASCADE,
    CONSTRAINT fk_hypothesis_edges_edge FOREIGN KEY (edge_id, investigation_id)
        REFERENCES edges (id, investigation_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_hypothesis_edges_edge
    ON hypothesis_edges (edge_id, hypothesis_id);

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

WITH updated AS (
    UPDATE version_investigations
    SET version_num = '202608280001'
    RETURNING version_investigations.version_num
)
INSERT INTO version_investigations (version_num)
SELECT '202608280001'
WHERE NOT EXISTS (SELECT 1 FROM updated)
RETURNING version_num;

COMMIT;
