-- Migration: 002 таблицы расследований
--
-- Локальной таблицы пользователей нет: субъекты приходят из auth платформы,
-- ссылки — *_subject_id (UUID). Расследования и под-расследования — одна
-- таблица с parent_id; отдельной сущности «находка» нет, гипотеза это
-- под-расследование со своим вердиктом.
--
-- Ссылки на SOM (issues, workspaces) внешними ключами не закрыты: другая база.
-- Целостность этих ссылок держит сервис.

BEGIN;

SET LOCAL search_path = inv, public, pg_temp;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = inv, pg_temp
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- СПРАВОЧНИКИ И КОНФИГУРАЦИЯ

CREATE TABLE IF NOT EXISTS sources (
    code VARCHAR(32) NOT NULL,

    kind VARCHAR(16) NOT NULL
        CHECK (kind IN ('siem', 'edr', 'ndr', 'infra', 'sandbox', 'other')),
    title VARCHAR NOT NULL,
    is_enabled BOOLEAN DEFAULT true NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_sources PRIMARY KEY (code)
);

DROP TRIGGER IF EXISTS trg_sources_set_updated_at ON sources;
CREATE TRIGGER trg_sources_set_updated_at
BEFORE UPDATE ON sources
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- Ядро (host, user, account, email, process, ip, domain, url, file_hash)
-- засевается миграцией; периферия вроде ja3 или registry_key добавляется
-- строкой справочника, без выкладки.
CREATE TABLE IF NOT EXISTS entity_types (
    code VARCHAR(64) NOT NULL,

    title VARCHAR NOT NULL,
    category VARCHAR(32) NOT NULL
        CHECK (category IN ('identity', 'network', 'execution', 'persistence', 'asset', 'other')),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_entity_types PRIMARY KEY (code)
);

CREATE TABLE IF NOT EXISTS relation_types (
    code VARCHAR(64) NOT NULL,

    title VARCHAR NOT NULL,
    source_kind VARCHAR(8) NOT NULL CHECK (source_kind IN ('entity', 'event')),
    target_kind VARCHAR(8) NOT NULL CHECK (target_kind IN ('entity', 'event')),
    directed BOOLEAN DEFAULT true NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_relation_types PRIMARY KEY (code)
);

-- ДЕРЕВО РАССЛЕДОВАНИЙ

CREATE TABLE IF NOT EXISTS investigations (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    parent_id UUID,

    title VARCHAR NOT NULL,
    description VARCHAR,
    status VARCHAR(8) DEFAULT 'open' NOT NULL
        CHECK (status IN ('open', 'closed')),
    severity VARCHAR(8)
        CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    -- Корень: incident | false_positive | not_affected | inconclusive.
    -- Под-расследование: confirmed | rejected | inconclusive.
    -- Подмножество по позиции в дереве проверяет сервис.
    verdict VARCHAR(16)
        CHECK (verdict IN ('incident', 'false_positive', 'not_affected',
                           'inconclusive', 'confirmed', 'rejected')),
    verdict_reason VARCHAR,
    confidence REAL CHECK (confidence >= 0 AND confidence <= 1),
    -- Кто создал: аналитик, детерминированное правило или агент.
    -- origin_ref уточняет — subject_id, код правила или идентификатор запуска.
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

-- Логическая связь с SOM. Внешний UUID хранится без FK, потому что SOM живёт в другой БД.
CREATE TABLE IF NOT EXISTS investigation_som_workspaces (
    investigation_id UUID NOT NULL,
    som_workspace_id UUID NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_som_workspaces PRIMARY KEY (investigation_id, som_workspace_id),
    CONSTRAINT fk_investigation_som_workspaces_investigation FOREIGN KEY (investigation_id)
        REFERENCES investigations (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_investigation_som_workspaces_workspace
    ON investigation_som_workspaces (som_workspace_id);

-- УЛИКИ
--
-- События и сущности принадлежат тенанту, а не расследованию: один и тот же
-- хост или одна и та же сработка штатно фигурируют в нескольких кейсах, и
-- копировать их на каждый — значит потерять ответ на вопрос «где ещё это
-- встречалось», ради которого карточка сущности и существует.
--
-- Принадлежность расследованию вынесена в investigation_events и
-- investigation_entities. Там же живёт провенанс: кто затянул и зачем.

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

    CONSTRAINT pk_events PRIMARY KEY (id),
    CONSTRAINT uq_events_id_project UNIQUE (id, project_id),
    -- Одна запись источника — одна строка на тенант. Затяжка в третий кейс
    -- ничего не копирует, только добавляет связь.
    CONSTRAINT uq_events_dedup UNIQUE (project_id, dedup_key),
    CONSTRAINT uq_events_source UNIQUE (project_id, source_code, source_event_id),
    CONSTRAINT fk_events_source_code_sources FOREIGN KEY (source_code)
        REFERENCES sources (code)
);

CREATE INDEX IF NOT EXISTS ix_events_timeline
    ON events (project_id, occurred_at, id);

CREATE INDEX IF NOT EXISTS ix_events_normalized
    ON events USING gin (normalized_data jsonb_path_ops);

-- Подстрочный поиск по командным строкам и значениям
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
    -- Тип и ключ опознают вещь в пределах тенанта. Через границу тенанта не
    -- склеиваются: dc-01 заказчика A и заказчика B — разные хосты.
    CONSTRAINT uq_entities_scope_type_key UNIQUE (project_id, type_code, canonical_key),
    CONSTRAINT fk_entities_type_code_entity_types FOREIGN KEY (type_code)
        REFERENCES entity_types (code)
);

DROP TRIGGER IF EXISTS trg_entities_set_updated_at ON entities;
CREATE TRIGGER trg_entities_set_updated_at
BEFORE UPDATE ON entities
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- Участие сущности в событии — факт источника, а не мнение расследования,
-- поэтому кейса здесь нет.
CREATE TABLE IF NOT EXISTS event_entity_relations (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    project_id VARCHAR(12) NOT NULL,
    event_id UUID NOT NULL,
    entity_id UUID NOT NULL,

    relation_code VARCHAR(64) NOT NULL,

    CONSTRAINT pk_event_entity_relations PRIMARY KEY (id),
    CONSTRAINT uq_event_entity_relations UNIQUE (event_id, entity_id, relation_code),
    -- Тенант в ключе с обеих сторон: без него событие одного заказчика
    -- связывалось бы с сущностью другого, и база бы это пропустила.
    CONSTRAINT fk_eer_event_id_events FOREIGN KEY (event_id, project_id)
        REFERENCES events (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_eer_entity_id_entities FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_eer_relation_code_relation_types FOREIGN KEY (relation_code)
        REFERENCES relation_types (code)
);

CREATE INDEX IF NOT EXISTS ix_eer_entity ON event_entity_relations (entity_id);
CREATE INDEX IF NOT EXISTS ix_eer_event ON event_entity_relations (event_id);

-- СОСТАВ РАССЛЕДОВАНИЯ

CREATE TABLE IF NOT EXISTS investigation_events (
    investigation_id UUID NOT NULL,
    event_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,

    attached_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    attached_by VARCHAR(16) DEFAULT 'analyst' NOT NULL
        CHECK (attached_by IN ('analyst', 'agent', 'system')),
    -- Зачем затянули: нарратив расследования, а не служебное поле.
    reason VARCHAR,

    CONSTRAINT pk_investigation_events PRIMARY KEY (investigation_id, event_id),
    CONSTRAINT uq_inv_events_tenant UNIQUE (investigation_id, event_id, project_id),
    CONSTRAINT fk_inv_events_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_inv_events_event FOREIGN KEY (event_id, project_id)
        REFERENCES events (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_inv_events_event ON investigation_events (event_id);

CREATE TABLE IF NOT EXISTS investigation_entities (
    investigation_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    project_id VARCHAR(12) NOT NULL,

    -- Как попала в кейс: извлечена из события, введена аналитиком как
    -- индикатор или предложена агентом.
    added_via VARCHAR(16) DEFAULT 'event' NOT NULL
        CHECK (added_via IN ('event', 'ioc', 'agent', 'analyst')),
    added_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_investigation_entities PRIMARY KEY (investigation_id, entity_id),
    CONSTRAINT uq_inv_entities_tenant UNIQUE (investigation_id, entity_id, project_id),
    CONSTRAINT fk_inv_entities_investigation FOREIGN KEY (investigation_id, project_id)
        REFERENCES investigations (id, project_id) ON DELETE CASCADE,
    CONSTRAINT fk_inv_entities_entity FOREIGN KEY (entity_id, project_id)
        REFERENCES entities (id, project_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_inv_entities_entity ON investigation_entities (entity_id);

-- ГРАФ
-- Tenant графа однозначно задаёт investigation_id; project_id здесь не дублируется.

CREATE TABLE IF NOT EXISTS graph_nodes (
    id UUID DEFAULT gen_random_uuid() NOT NULL,
    investigation_id UUID NOT NULL,

    node_type VARCHAR(8) NOT NULL CHECK (node_type IN ('entity', 'event')),
    entity_id UUID,
    event_id UUID,
    origin VARCHAR(8) NOT NULL CHECK (origin IN ('analyst', 'rule', 'agent')),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_graph_nodes PRIMARY KEY (id),
    CONSTRAINT uq_graph_nodes_id_investigation UNIQUE (id, investigation_id),
    -- Ровно одна ссылка, и она соответствует объявленному типу узла
    CONSTRAINT ck_graph_nodes_target CHECK (
        (node_type = 'entity' AND entity_id IS NOT NULL AND event_id IS NULL) OR
        (node_type = 'event'  AND event_id  IS NOT NULL AND entity_id IS NULL)
    ),
    CONSTRAINT fk_graph_nodes_investigation_id_investigations FOREIGN KEY (investigation_id)
        REFERENCES investigations (id) ON DELETE CASCADE,
    -- Ссылка не на общую строку, а на её принадлежность этому расследованию:
    -- на граф кейса не попадёт то, что в кейс не затянуто, а отвязка события
    -- унесёт узел и висящие на нём рёбра.
    CONSTRAINT fk_graph_nodes_entity FOREIGN KEY (investigation_id, entity_id)
        REFERENCES investigation_entities (investigation_id, entity_id) ON DELETE CASCADE,
    CONSTRAINT fk_graph_nodes_event FOREIGN KEY (investigation_id, event_id)
        REFERENCES investigation_events (investigation_id, event_id) ON DELETE CASCADE
);

-- Одна сущность (событие) — один узел в расследовании, иначе рёбра расщепятся
CREATE UNIQUE INDEX IF NOT EXISTS uq_graph_nodes_entity
    ON graph_nodes (investigation_id, entity_id) WHERE entity_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_graph_nodes_event
    ON graph_nodes (investigation_id, event_id) WHERE event_id IS NOT NULL;

-- Узел может быть связан с одной или несколькими задачами SOM.
-- Целостность som_issue_id проверяет интеграционный слой: межбазового FK здесь быть не может.
CREATE TABLE IF NOT EXISTS graph_node_som_issues (
    graph_node_id UUID NOT NULL,
    som_issue_id UUID NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_graph_node_som_issues PRIMARY KEY (graph_node_id, som_issue_id),
    CONSTRAINT fk_graph_node_som_issues_node FOREIGN KEY (graph_node_id)
        REFERENCES graph_nodes (id) ON DELETE CASCADE
);

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
    -- Обоснование связи. Для origin=agent обязательно — проверяет сервис.
    why VARCHAR,
    origin VARCHAR(8) NOT NULL CHECK (origin IN ('analyst', 'rule', 'agent')),
    -- Кто именно: subject_id аналитика, код правила или идентификатор запуска
    origin_ref VARCHAR,
    metadata JSONB DEFAULT '{}'::jsonb NOT NULL,
    version INTEGER DEFAULT 1 NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_edges PRIMARY KEY (id),
    CONSTRAINT uq_edges_id_investigation UNIQUE (id, investigation_id),
    -- Идемпотентность: повторный прогон правил и агента не плодит дубли
    CONSTRAINT uq_edges_triple UNIQUE (investigation_id, source_node_id, target_node_id, relation_code),
    CONSTRAINT ck_edges_rejected_reason
        CHECK (status <> 'rejected' OR reject_reason IS NOT NULL),
    CONSTRAINT fk_edges_investigation_id_investigations FOREIGN KEY (investigation_id)
        REFERENCES investigations (id) ON DELETE CASCADE,
    -- Составными, а не по одному id: иначе ребро одного расследования могло бы
    -- связать узлы другого — обе строки валидны по отдельности, и заметить это
    -- было бы нечем. Ключ uq_graph_nodes_id_investigation заведён ровно под это.
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

-- Основания связи. Инвариант тот же — цитируемое событие принадлежит тому же
-- расследованию, что и ребро, — но держится теперь через состав кейса:
-- события общие на тенант, «своим» его делает запись в investigation_events.
-- Отвязали событие от кейса — основания в нём отвалились вместе с ним.
CREATE TABLE IF NOT EXISTS edge_evidence (
    edge_id UUID NOT NULL,
    event_id UUID NOT NULL,
    investigation_id UUID NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_edge_evidence PRIMARY KEY (edge_id, event_id),
    CONSTRAINT fk_edge_evidence_edge FOREIGN KEY (edge_id, investigation_id)
        REFERENCES edges (id, investigation_id) ON DELETE CASCADE,
    CONSTRAINT fk_edge_evidence_event FOREIGN KEY (investigation_id, event_id)
        REFERENCES investigation_events (investigation_id, event_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_edge_evidence_event ON edge_evidence (event_id);

-- ДОСТУП

CREATE TABLE IF NOT EXISTS role_bindings (
    project_id VARCHAR(12) NOT NULL,
    subject_id UUID NOT NULL,
    role VARCHAR(8) NOT NULL CHECK (role IN ('l1', 'l2', 'lead', 'admin')),

    granted_by_subject_id UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_role_bindings PRIMARY KEY (project_id, subject_id, role)
);

CREATE INDEX IF NOT EXISTS ix_role_bindings_subject ON role_bindings (subject_id);

COMMIT;
