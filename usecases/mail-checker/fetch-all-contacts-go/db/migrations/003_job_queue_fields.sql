alter table runs
  add column if not exists stop_page int null;

alter table runs
  add column if not exists completed_at timestamptz null;

alter table pages
  add column if not exists next_attempt_at timestamptz not null default now();

alter table pages
  add column if not exists locked_by text null;

alter table pages
  add column if not exists locked_at timestamptz null;

create index if not exists idx_pages_run_status_attempt_page
  on pages(run_id, status, next_attempt_at, page_number);
