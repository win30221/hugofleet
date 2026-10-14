ALTER TABLE applications ADD COLUMN observed_snapshot TEXT NOT NULL DEFAULT '{}';
ALTER TABLE operations ADD COLUMN traceparent TEXT NOT NULL DEFAULT '';
ALTER TABLE operations ADD COLUMN tracestate TEXT NOT NULL DEFAULT '';
