-- Migration: add batch tracking to pages and contact_keys
-- Run this after your existing schema is in place.

-- ── page_batches ─────────────────────────────────────────────────────────────
-- One row per batch claim. A batch is a group of pages claimed together in a
-- single ClaimNextPages call. The batch_id is the primary key used for resume.
create table if not exists page_batches (
    id         bigserial   primary key,
    run_id     uuid        not null references runs(id),
    worker_id  text,
    created_at timestamptz not null default now()
);

create index if not exists page_batches_run_id_idx on page_batches(run_id);

-- ── pages: add batch_id column ───────────────────────────────────────────────
alter table pages
    add column if not exists batch_id bigint references page_batches(id);

create index if not exists pages_batch_id_idx on pages(batch_id)
    where batch_id is not null;

-- ── contact_keys: add first_seen_batch_id column ────────────────────────────
alter table contact_keys
    add column if not exists first_seen_batch_id bigint references page_batches(id);

create index if not exists contact_keys_batch_id_idx on contact_keys(first_seen_batch_id)
    where first_seen_batch_id is not null;

-- ── runs: replace last_exit_page with last_exit_batch ───────────────────────
-- We keep last_exit_page for backward compat but add last_exit_batch as the
-- preferred resume anchor.
alter table runs
    add column if not exists last_exit_batch bigint;
