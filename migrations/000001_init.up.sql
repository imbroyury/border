CREATE TABLE zones (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    border VARCHAR(10) NOT NULL
);

CREATE TABLE snapshots (
    id BIGSERIAL PRIMARY KEY,
    zone_id VARCHAR(50) NOT NULL REFERENCES zones(id),
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cars_count INT NOT NULL DEFAULT 0,
    sent_last_hour INT NOT NULL DEFAULT 0,
    sent_last_24h INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_snapshots_zone_captured ON snapshots(zone_id, captured_at);

CREATE TABLE vehicles (
    id BIGSERIAL PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    zone_id VARCHAR(50) NOT NULL,
    reg_number VARCHAR(20) NOT NULL,
    queue_type VARCHAR(50) NOT NULL DEFAULT '',
    registered_at TIMESTAMPTZ,
    status_changed_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT ''
);

CREATE INDEX idx_vehicles_snapshot ON vehicles(snapshot_id);
CREATE INDEX idx_vehicles_zone_registered ON vehicles(zone_id, registered_at);
