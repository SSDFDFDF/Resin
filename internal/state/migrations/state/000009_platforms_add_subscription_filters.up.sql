ALTER TABLE platforms ADD COLUMN subscription_filters_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE platforms ADD COLUMN subscription_filter_invert INTEGER NOT NULL DEFAULT 0;
