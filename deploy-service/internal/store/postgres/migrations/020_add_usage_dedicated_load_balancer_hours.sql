ALTER TABLE resource_usage
ADD COLUMN IF NOT EXISTS dedicated_load_balancer_hours FLOAT NOT NULL DEFAULT 0;
