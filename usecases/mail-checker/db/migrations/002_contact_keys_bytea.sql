-- Store contact_key as BYTEA so it can accept any bytes.
-- Existing text values are converted using UTF-8 encoding.

alter table contact_keys
  alter column contact_key type bytea
  using convert_to(contact_key, 'UTF8');

