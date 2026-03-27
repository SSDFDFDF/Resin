ALTER TABLE platforms
ADD COLUMN sticky_lease_mode TEXT NOT NULL DEFAULT 'TTL';

ALTER TABLE platforms
ADD COLUMN manual_unavailable_action TEXT NOT NULL DEFAULT 'HOLD';

ALTER TABLE platforms
ADD COLUMN manual_unavailable_grace_ns INTEGER NOT NULL DEFAULT 0;
