#!/bin/bash
# fix_migrations.sh - Manually create analytics tables

echo "Creating analytics tables..."

docker-compose exec -T postgres psql -U urlshortener -d urlshortener <<'EOF'
-- Create clicks table for raw event data
CREATE TABLE IF NOT EXISTS clicks (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) NOT NULL,
    clicked_at TIMESTAMP NOT NULL DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    referer TEXT,
    country VARCHAR(2)
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_clicks_short_code ON clicks(short_code);
CREATE INDEX IF NOT EXISTS idx_clicks_clicked_at ON clicks(clicked_at);
CREATE INDEX IF NOT EXISTS idx_clicks_ip_address ON clicks(ip_address);

-- Create url_stats table for aggregated statistics
CREATE TABLE IF NOT EXISTS url_stats (
    short_code VARCHAR(10) PRIMARY KEY,
    total_clicks BIGINT NOT NULL DEFAULT 0,
    unique_visitors BIGINT NOT NULL DEFAULT 0,
    last_clicked_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Index for efficient updates
CREATE INDEX IF NOT EXISTS idx_url_stats_updated_at ON url_stats(updated_at);

-- Verify tables were created
\dt
EOF

echo "Done! Checking tables..."
docker-compose exec postgres psql -U urlshortener -d urlshortener -c "\dt"
