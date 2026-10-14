ALTER TABLE applications ADD COLUMN observation_error_code TEXT NOT NULL DEFAULT '';
ALTER TABLE applications ADD COLUMN observation_error_message TEXT NOT NULL DEFAULT '';

-- Lifecycle events remain once per operation. Observed failures and recoveries
-- may recur, but are inserted only on a change of the persisted observation.
ALTER TABLE operation_events RENAME TO old_operation_events;
CREATE TABLE operation_events(id TEXT PRIMARY KEY,operation_id TEXT NOT NULL REFERENCES operations(id),kind TEXT NOT NULL,message TEXT NOT NULL,occurred_at INTEGER NOT NULL);
INSERT INTO operation_events SELECT * FROM old_operation_events;
DROP TABLE old_operation_events;
CREATE UNIQUE INDEX one_lifecycle_event ON operation_events(operation_id,kind) WHERE kind NOT IN ('OBSERVED_DEGRADED','OBSERVED_RECOVERED');
CREATE INDEX events_by_operation ON operation_events(operation_id,occurred_at);
