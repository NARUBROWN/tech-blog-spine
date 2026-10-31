CREATE TABLE IF NOT EXISTS analytics_performance (
 id CHAR(36) NOT NULL,
 visit_id CHAR(36) NOT NULL,
 metric VARCHAR(24) NOT NULL,
 endpoint VARCHAR(120) NOT NULL DEFAULT '',
 duration_ms DECIMAL(10,3) NOT NULL,
 status_code SMALLINT NOT NULL DEFAULT 0,
 revision INT NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 PRIMARY KEY (visit_id,id),
 KEY idx_performance_date (created_at,metric),
 FOREIGN KEY (visit_id) REFERENCES analytics_visit(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
