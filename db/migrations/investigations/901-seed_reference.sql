-- Ядро типов сущностей, связей и два реально поддержанных PT-источника.

BEGIN;

SET LOCAL search_path = :"DB_INV_SCHEMA_NAME", pg_temp;

INSERT INTO entity_types (code, title, category) VALUES
    ('host',      'Узел',           'asset'),
    ('user',      'Пользователь',   'identity'),
    ('account',   'Учетная запись', 'identity'),
    ('email',     'Email',          'identity'),
    ('process',   'Процесс',        'execution'),
    ('ip',        'IP-адрес',       'network'),
    ('mac',       'MAC-адрес',      'network'),
    ('hostname',  'Имя узла',       'asset'),
    ('domain',    'Домен',          'network'),
    ('url',       'URL',            'network'),
    ('file_hash', 'Файл / хеш',     'execution'),
    ('hash',      'Хеш',            'execution')
ON CONFLICT (code) DO NOTHING;

-- Роли сущности в событии
INSERT INTO relation_types (code, title, source_kind, target_kind, directed) VALUES
    ('mentions', 'Упоминает',       'event',  'entity', true),
    ('actor',    'Инициатор',       'event',  'entity', true),
    ('object',   'Объект',          'event',  'entity', true),
    ('src',      'Источник',        'event',  'entity', true),
    ('dst',      'Назначение',      'event',  'entity', true),
    ('attacker', 'Атакующий',       'event',  'entity', true),
    ('victim',   'Жертва',          'event',  'entity', true),
    ('account',  'Учетная запись',  'event',  'entity', true),
    ('file',     'Файл',             'event',  'entity', true)
ON CONFLICT (code) DO NOTHING;

-- Связи между сущностями
INSERT INTO relation_types (code, title, source_kind, target_kind, directed) VALUES
    ('parent_process', 'Родительский процесс', 'entity', 'entity', true),
    ('logged_in',      'Вход на узел',         'entity', 'entity', true),
    ('connected_to',   'Сетевое соединение',   'entity', 'entity', true),
    ('has_interface',  'Сетевой интерфейс',    'entity', 'entity', true),
    ('authenticated_to','Аутентификация',       'entity', 'entity', true),
    ('transferred_to', 'Передача данных',       'entity', 'entity', true),
    ('executed',       'Запуск файла',         'entity', 'entity', true),
    ('resolved_to',    'Резолв домена',        'entity', 'entity', true),
    ('same_host',      'Тот же узел',          'entity', 'entity', false)
ON CONFLICT (code) DO NOTHING;

-- Связи между событиями: цепочка атаки во времени
INSERT INTO relation_types (code, title, source_kind, target_kind, directed) VALUES
    ('subevent_of', 'Породило сработку', 'event', 'event', true),
    ('followed_by', 'Следующее событие', 'event', 'event', true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO sources (code, kind, title) VALUES
    ('pt-maxpatrol-siem', 'siem', 'MaxPatrol SIEM'),
    ('pt-nad',            'ndr',  'PT Network Attack Discovery')
ON CONFLICT (code) DO NOTHING;

COMMIT;
