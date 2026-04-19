create table if not exists validation_runs (
  id uuid primary key default gen_random_uuid(),
  source_file text not null,
  total_rows int not null default 0,
  state text not null default 'pending',
  created_at timestamptz not null default now(),
  started_at timestamptz null,
  completed_at timestamptz null
);

create table if not exists validation_results (
  id bigserial primary key,
  run_id uuid not null references validation_runs(id) on delete cascade,
  row_number int not null,
  contact_id text not null,
  raw_contact_key text not null,
  clean_candidate text null,
  normalized_email text null,
  status text not null default 'pending',
  failure_reason text null,
  syntax_status text null,
  syntax_reason text null,
  syntax_latency_ms int not null default 0,
  syntax_score int not null default 0,
  domain_dns_status text null,
  domain_dns_reason text null,
  domain_dns_latency_ms int not null default 0,
  domain_dns_score int not null default 0,
  mx_status text null,
  mx_reason text null,
  mx_latency_ms int not null default 0,
  mx_score int not null default 0,
  smtp_status text null,
  smtp_reason text null,
  smtp_latency_ms int not null default 0,
  smtp_score int not null default 0,
  history_status text null,
  history_reason text null,
  history_score int not null default 0,
  history_payload jsonb null,
  history_fetched_at timestamptz null,
  total_score int not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(run_id, row_number)
);

create index if not exists idx_validation_results_run_row
  on validation_results(run_id, row_number);

create index if not exists idx_validation_results_run_status
  on validation_results(run_id, status);
