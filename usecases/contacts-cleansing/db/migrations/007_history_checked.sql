-- Add history_checked_at to track which contacts have already had their
-- message history fetched. NULL = not yet checked; non-NULL = already done.
-- This makes the history processor fully resumable.
ALTER TABLE contact_keys
    ADD COLUMN IF NOT EXISTS history_checked_at timestamptz NULL;
 
-- Index so GetContactsForHistory (WHERE history_checked_at IS NULL) is fast
-- even on large tables.
CREATE INDEX IF NOT EXISTS contact_keys_history_unchecked_idx
    ON contact_keys (contact_id)
    WHERE history_checked_at IS NULL;
 