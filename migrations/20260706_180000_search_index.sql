-- UPGRADE
-- Content search index.
--
-- Why pg_trgm rather than tsvector: PostgreSQL's built-in text search has no
-- Chinese tokenizer, so `to_tsvector('simple', '季度合同模板')` yields one useless
-- token. pg_trgm indexes character trigrams, which works for Chinese and English
-- alike, and — crucially — makes `ILIKE '%...%'` index-backed instead of a full
-- table scan. It ships with postgresql-contrib, so self-hosting cost is one
-- `CREATE EXTENSION`.
--
-- Ceiling: trigram lookups need at least 3 characters; shorter queries still
-- scan. Multi-byte characters count individually, so a 3-character Chinese
-- query is enough.

create extension if not exists pg_trgm;

create table if not exists search_index
(
    file_id    bigint primary key
        references file_items
            on delete cascade,
    user_id    bigint      not null,
    content    text        not null, -- all searchable text, pre-joined for this file
    updated_at timestamp   not null default now()
);

create index if not exists idx_search_index_content_trgm
    on search_index using gin (content gin_trgm_ops);

-- Queries are always scoped to one user.
create index if not exists idx_search_index_user_id
    on search_index (user_id);

-- DOWNGRADE
drop index if exists idx_search_index_user_id;
drop index if exists idx_search_index_content_trgm;
drop table if exists search_index;
-- The pg_trgm extension is intentionally left installed: other objects may use it.
