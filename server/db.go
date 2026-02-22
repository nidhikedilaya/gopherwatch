package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql" // Import the MySQL driver
)

// InitDB opens a connection pool to the MySQL database
func InitDB(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	slog.Info("Successfully connected to MySQL database")
	return db
}

// SaveAlert inserts a triggered alert into the database
func SaveAlert(db *sql.DB, serviceName, metric string, value float64) {
	// SQL Best Practice: Use context.WithTimeout to prevent the alerting engine from hanging
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `INSERT INTO alerts (service_name, metric, metric_value) VALUES (?, ?, ?)`

	_, err := db.ExecContext(ctx, query, serviceName, metric, value)
	if err != nil {
		slog.Error("Failed to save alert to database", "error", err)
		return
	}

	slog.Debug("Alert saved to database successfully", "service", serviceName)
}
