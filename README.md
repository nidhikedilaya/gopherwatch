# GopherWatch
Distributed Metrics Monitoring & Alerting System (Go | gRPC | REST | MySQL)
GopherWatch is a lightweight internal monitoring system designed to collect hardware metrics from distributed microservices via gRPC streaming, evaluate them concurrently using a worker pool, and expose a REST API to view service health and alert history.

# Features
✓ gRPC Client → Server Streaming
Agents continuously stream CPU, Memory, Request Count, etc.
Server handles multiple concurrent streaming agents.
Interceptors log Service-ID from metadata.
✓ Concurrent Alerting Engine
Fan-out worker pool evaluates all metrics in parallel.
Alerts raised when threshold rules are violated.
Thread-safe state tracking using sync.Map or RWMutex.
CRITICAL alerts logged using log/slog.
✓ REST API Dashboard
GET /status → current state of all services
GET /alerts/history → historical alerts from database
JSON clean outputs with proper HTTP codes.
✓ SQL Integration
Alerts are persisted in MySQL.
Uses context.WithTimeout for safe DB operations.
Indexed by service_name + timestamp.
✓ Graceful Shutdown
gRPC shutdown
REST shutdown
DB connection pool close
Context propagation everywhere

# Installation & Setup
## Prerequisites
- Go 1.20+
- Protoc compiler
- MySQL running locally

## Install gRPC tools:
```
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

## Database Setup
```
-- 1. Create the database
CREATE DATABASE IF NOT EXISTS gopherwatch;

-- 2. Tell MySQL you want to use this new database
USE gopherwatch;

-- 3. Create the alerts table with the required schema and indexes
CREATE TABLE alerts (
    id INT AUTO_INCREMENT PRIMARY KEY,
    service_name VARCHAR(255) NOT NULL,
    metric VARCHAR(50) NOT NULL,
    metric_value FLOAT NOT NULL,
    triggered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_service_time (service_name, triggered_at)
);
```
Store your database credentials in your .env file.

## Running the Project

Start the Metrics + REST Server
```go run server/*.go```

Start the Front-End
```npm run dev```

Run the Agent Client
```go run client/main.go```
This begins streaming metrics to the gRPC server.

Test the REST API
Get current status
```curl http://localhost:8080/status```
Get alert history
```curl http://localhost:8080/alerts/history```

## gRPC Interface
From metrics.proto:
```
service MetricsService {
  rpc SendMetrics (stream MetricReport) returns (Summary);
}
```

The server:
- Receives a continuous stream
- Extracts service metadata
- Dispatches metrics to workers

## Worker Pool & Alerting
- Metrics are pushed to a Dispatcher Channel.
- Worker goroutines continuously read from the channel.
- Each worker compares metrics to predefined rules:
    e.g., CPU > 90%, Memory > 80%

On violation:
- Log CRITICAL alert
- Insert record into MySQL
- Update global state

## Graceful Shutdown
- Cancels context for all DB operations
- Closes gRPC server
- Stops REST server with timeout
- Ensures no new DB writes occur after shutdown begins
