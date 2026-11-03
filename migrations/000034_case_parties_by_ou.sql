-- Created at: 2026-11-03T09:30:00Z
-- Need Postgres- or SQLite-only SQL? Nest -- @postgres / -- @sqlite blocks
-- inside @UP or @DOWN -- see docs/migrations.md.

-- @UP
-- A case party is named by its OU handle (party_type 'ou'), the ouHandle claim its users'
-- tokens carry, instead of a company_records id, so an agency's own OU can own its cases
-- without a company record. The company rows are deleted, not converted: consignments
-- created before this lose their trader and CHA owners. See internal/caseparty.
DELETE FROM case_parties WHERE party_type = 'company';

ALTER TABLE case_parties DROP CONSTRAINT IF EXISTS case_parties_party_type_check;
ALTER TABLE case_parties ADD CONSTRAINT case_parties_party_type_check CHECK (party_type IN ('ou', 'user'));

-- @DOWN
DELETE FROM case_parties WHERE party_type = 'ou';

ALTER TABLE case_parties DROP CONSTRAINT IF EXISTS case_parties_party_type_check;
ALTER TABLE case_parties ADD CONSTRAINT case_parties_party_type_check CHECK (party_type IN ('company', 'user'));
