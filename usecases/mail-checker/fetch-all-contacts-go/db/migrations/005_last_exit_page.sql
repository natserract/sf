-- Track the highest page touched when a worker exits so resume knows where
-- to continue.  NULL means the run has never been interrupted.
alter table runs
  add column if not exists last_exit_page int null;
