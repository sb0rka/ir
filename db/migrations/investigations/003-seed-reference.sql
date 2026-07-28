-- Migration: 003 справочники и демо-конфигурация
--
-- Ядро типов сущностей и базовые типы связей — без них не построить граф.
-- Источники засеяны четырьмя классами: SIEM, EDR, NDR, инфраструктурные логи.

BEGIN;

SET LOCAL search_path = inv, public, pg_temp;

INSERT INTO entity_types (code, title, category) VALUES
    ('host',      'Узел',           'asset'),
    ('user',      'Пользователь',   'identity'),
    ('account',   'Учетная запись', 'identity'),
    ('email',     'Email',          'identity'),
    ('process',   'Процесс',        'execution'),
    ('ip',        'IP-адрес',       'network'),
    ('domain',    'Домен',          'network'),
    ('url',       'URL',            'network'),
    ('file_hash', 'Файл / хеш',     'execution')
ON CONFLICT (code) DO NOTHING;

-- Роли сущности в событии
INSERT INTO relation_types (code, title, source_kind, target_kind, directed) VALUES
    ('actor',   'Инициатор',      'entity', 'event',  true),
    ('object',  'Объект',         'entity', 'event',  true),
    ('src',     'Источник',       'entity', 'event',  true),
    ('dst',     'Назначение',     'entity', 'event',  true)
ON CONFLICT (code) DO NOTHING;

-- Связи между сущностями
INSERT INTO relation_types (code, title, source_kind, target_kind, directed) VALUES
    ('parent_process', 'Родительский процесс', 'entity', 'entity', true),
    ('logged_in',      'Вход на узел',         'entity', 'entity', true),
    ('connected_to',   'Сетевое соединение',   'entity', 'entity', true),
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
    ('siem',  'siem',  'SIEM (демо-датасет)'),
    ('edr',   'edr',   'EDR (демо-датасет)'),
    ('ndr',   'ndr',   'NDR (демо-датасет)'),
    ('infra', 'infra', 'Инфраструктурные логи (демо-датасет)')
ON CONFLICT (code) DO NOTHING;

COMMIT;
