-- Runs represent a logical fetch session.
create extension if not exists pgcrypto;

create table if not exists runs (
  id uuid primary key default gen_random_uuid(),
  created_at timestamptz not null default now(),
  state text not null default 'running',
  page_size int not null,
  started_page int not null default 1,
  base_url text not null,
  filter_operator text not null,
  filter_value text not null,
  total_contacts int not null default 0
);

-- Pages represent per-page durable progress tracking.
create table if not exists pages (
  run_id uuid not null references runs(id) on delete cascade,
  page_number int not null,
  status text not null default 'pending', -- pending|in_progress|done|failed|empty
  attempts int not null default 0,
  last_error text null,
  started_at timestamptz null,
  finished_at timestamptz null,
  updated_at timestamptz not null default now(),
  primary key (run_id, page_number)
);

-- contact_keys stores unique extracted contact keys (trimmed).
create table if not exists contact_keys (
  contact_key text primary key,
  contact_id text not null,
  first_seen_run_id uuid not null references runs(id) on delete restrict,
  first_seen_page int not null,
  data jsonb null,
  inserted_at timestamptz not null default now()
);

create index if not exists idx_pages_run_status on pages(run_id, status);

ALTER TABLE contact_keys 
ADD CONSTRAINT contact_keys_unique UNIQUE (contact_key, contact_id);