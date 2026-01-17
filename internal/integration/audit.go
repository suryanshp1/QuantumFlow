package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteAuditLogger implements audit logging using SQLite
type SQLiteAuditLogger struct {
	db *sql.DB
}

// NewSQLiteAuditLogger creates a new SQLite audit logger
func NewSQLiteAuditLogger(dbPath string) (*SQLiteAuditLogger, error) {
	// Expand path
	if strings.HasPrefix(dbPath, "~/") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[2:])
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	logger := &SQLiteAuditLogger{db: db}

	// Initialize schema
	if err := logger.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return logger, nil
}

// initSchema creates the audit log table
func (a *SQLiteAuditLogger) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		service TEXT NOT NULL,
		operation TEXT NOT NULL,
		user_id TEXT,
		request_id TEXT,
		method TEXT,
		endpoint TEXT,
		status_code INTEGER,
		duration_ms INTEGER,
		success BOOLEAN,
		error TEXT,
		metadata TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_service ON audit_log(service);
	CREATE INDEX IF NOT EXISTS idx_user_id ON audit_log(user_id);
	`

	_, err := a.db.Exec(schema)
	return err
}

// Log records an API call
func (a *SQLiteAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	query := `
		INSERT INTO audit_log (
			timestamp, service, operation, user_id, request_id,
			method, endpoint, status_code, duration_ms, success, error, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	metadataJSON := "{}"
	if entry.Metadata != nil {
		// Would serialize metadata to JSON
		_ = entry.Metadata
	}

	_, err := a.db.ExecContext(ctx, query,
		entry.Timestamp,
		entry.Service,
		entry.Operation,
		entry.UserID,
		entry.RequestID,
		entry.Method,
		entry.Endpoint,
		entry.StatusCode,
		entry.Duration.Milliseconds(),
		entry.Success,
		entry.Error,
		metadataJSON,
	)

	return err
}

// Query retrieves audit logs
func (a *SQLiteAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	query := "SELECT id, timestamp, service, operation, user_id, method, endpoint, status_code, duration_ms, success, error FROM audit_log WHERE 1=1"
	args := []interface{}{}

	if filter.Service != nil {
		query += " AND service = ?"
		args = append(args, string(*filter.Service))
	}

	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartTime)
	}

	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndTime)
	}

	if filter.UserID != nil {
		query += " AND user_id = ?"
		args = append(args, *filter.UserID)
	}

	if filter.Success != nil {
		query += " AND success = ?"
		args = append(args, *filter.Success)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var id int64
		var durationMs int64

		err := rows.Scan(
			&id,
			&entry.Timestamp,
			&entry.Service,
			&entry.Operation,
			&entry.UserID,
			&entry.Method,
			&entry.Endpoint,
			&entry.StatusCode,
			&durationMs,
			&entry.Success,
			&entry.Error,
		)

		if err != nil {
			return nil, err
		}

		entry.Duration = time.Duration(durationMs) * time.Millisecond
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// Close closes the database connection
func (a *SQLiteAuditLogger) Close() error {
	return a.db.Close()
}

// GetStats returns audit statistics
func (a *SQLiteAuditLogger) GetStats(ctx context.Context, service ServiceType, since time.Time) (*AuditStats, error) {
	query := `
		SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful,
			AVG(duration_ms) as avg_duration_ms
		FROM audit_log 
		WHERE service = ? AND timestamp >= ?
	`

	var stats AuditStats
	var avgDuration sql.NullFloat64

	err := a.db.QueryRowContext(ctx, query, service, since).Scan(
		&stats.TotalRequests,
		&stats.SuccessfulRequests,
		&avgDuration,
	)

	if err != nil {
		return nil, err
	}

	if avgDuration.Valid {
		stats.AverageDuration = time.Duration(avgDuration.Float64) * time.Millisecond
	}

	if stats.TotalRequests > 0 {
		stats.ErrorRate = float64(stats.TotalRequests-stats.SuccessfulRequests) / float64(stats.TotalRequests)
	}

	return &stats, nil
}

// AuditStats holds audit statistics
type AuditStats struct {
	TotalRequests      int
	SuccessfulRequests int
	ErrorRate          float64
	AverageDuration    time.Duration
}
