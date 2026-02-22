package main

import (
	"context"
	"database/sql" // NEW: Required for sql.DB
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "gopherwatch/proto"

	_ "github.com/go-sql-driver/mysql" // NEW: Import the MySQL driver anonymously
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AlertHistory represents a past alert for the REST API
type AlertHistory struct {
	ID          int       `json:"id"`
	ServiceName string    `json:"service_name"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// handleGetStatus returns the live health of all registered services
func handleGetStatus(engine *Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Thread-safe read from the global state map
		engine.mu.RLock()
		defer engine.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) //  Returns proper HTTP status codes (200 OK)

		json.NewEncoder(w).Encode(engine.state)
	}
}

// handleGetAlertsHistory returns past alerts from MySQL
func handleGetAlertsHistory(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Context Propagation: Pass-through of timeout from the REST layer to the DB layer
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		rows, err := db.QueryContext(ctx, "SELECT id, service_name, metric, metric_value, triggered_at FROM alerts ORDER BY triggered_at DESC LIMIT 50")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to query database"})
			return
		}
		defer rows.Close()

		var history []AlertHistory
		for rows.Next() {
			var alert AlertHistory
			if err := rows.Scan(&alert.ID, &alert.ServiceName, &alert.Metric, &alert.Value, &alert.TriggeredAt); err != nil {
				continue
			}
			history = append(history, alert)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(history)
	}
}

// metricsServer implements the gRPC service and holds a reference to the Engine
type metricsServer struct {
	pb.UnimplementedMetricsServiceServer
	engine *Engine // The engine handles the dispatcher channel and state tracking
}

// RegisterAgent handles the unary request
func (s *metricsServer) RegisterAgent(ctx context.Context, info *pb.AgentInfo) (*pb.ConfigResponse, error) {
	// Extract the Service-ID from the context just like we do in the stream
	md, ok := metadata.FromIncomingContext(ctx)
	serviceID := "unknown-service"
	if ok {
		if ids := md.Get("service-id"); len(ids) > 0 {
			serviceID = ids[0]
		}
	}

	slog.Info("Agent Registered via Unary RPC",
		"service_id", serviceID,
		"os", info.OsVersion,
		"agent", info.AgentVersion)

	// Send back a configuration telling the agent to stream every 500 milliseconds
	return &pb.ConfigResponse{
		ReportIntervalMs: 500,
		Active:           true,
	}, nil
}

// SendMetrics receives the stream and pushes metrics to the Dispatcher channel
func (s *metricsServer) SendMetrics(stream pb.MetricsService_SendMetricsServer) error {
	// Extract Service-ID from context (set by your interceptor)
	md, ok := metadata.FromIncomingContext(stream.Context())
	serviceID := "unknown-service"
	if ok {
		if ids := md.Get("service-id"); len(ids) > 0 {
			serviceID = ids[0]
		}
	}

	var reportCount int32 = 0

	for {
		report, err := stream.Recv()

		// Correctly handle io.EOF and stream errors
		if err == io.EOF {
			slog.Info("Stream closed by client", "service_id", serviceID)
			s.engine.RemoveService(serviceID)
			return stream.SendAndClose(&pb.Summary{
				TotalReportsReceived: reportCount,
				Status:               "SUCCESS",
			})
		}
		if err != nil {
			slog.Error("Stream error", "error", err)
			return err
		}

		reportCount++

		// FAN-OUT PATTERN: Send the metric to the Dispatcher channel
		// This prevents one slow rule evaluation from backing up the whole metrics stream
		s.engine.dispatcher <- ServiceMetric{
			ServiceID: serviceID,
			Report:    report,
		}
	}
}

func main() {
	// --- NEW: DATABASE CONNECTION ---
	// Initialize Database Connection using your credentials
	// 2. Fetch the credentials
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found...")
	}
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	// 3. Construct the DSN dynamically
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local",
		dbUser, dbPass, dbHost, dbPort, dbName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close() // Drains the DB connection pool properly when main exits

	// Ping verifies if the database is actually reachable
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	slog.Info("Connected to MySQL successfully")
	// --------------------------------

	// 1. Initialize threshold rules (e.g., If CPU > 90%)
	rules := []AlertRule{
		{Metric: "CPU", Threshold: 90.0},
		{Metric: "MEMORY", Threshold: 8000.0}, // An additional example rule
	}

	// 2. Initialize Engine and Worker Pool (Now passing the DB!)
	engine := NewEngine(rules, 100, db)
	engine.StartWorkers(5) // Multiple worker goroutines pull from the channel concurrently

	router := mux.NewRouter()
	router.HandleFunc("/status", handleGetStatus(engine)).Methods(http.MethodGet)
	router.HandleFunc("/alerts/history", handleGetAlertsHistory(db)).Methods(http.MethodGet)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	// Run the HTTP server in a separate background goroutine so it doesn't block gRPC
	go func() {
		slog.Info("REST API Dashboard listening on port 8080")
		if err := http.ListenAndServe(":8080", router); err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 3. Setup TCP Listener
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// 4. Create gRPC Server with your existing interceptor
	grpcServer := grpc.NewServer(
		grpc.StreamInterceptor(StreamAuthInterceptor),
	)

	// 5. Register the service with the engine attached
	pb.RegisterMetricsServiceServer(grpcServer, &metricsServer{engine: engine})

	go func() {
		slog.Info("gRPC server listening on port 50051 with 5 workers running")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// The main thread blocks here until a signal is received
	<-quit
	slog.Info("Shutdown signal received. Shutting down gracefully...")

	// 5a. Shut down the REST API with a 5-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown forced", "error", err)
	} else {
		slog.Info("HTTP server stopped cleanly")
	}

	// 5b. Shut down the gRPC server
	// GracefulStop waits for all pending RPCs and streams to finish
	grpcServer.GracefulStop()
	slog.Info("gRPC server stopped cleanly")

	// 5c. Drain the database connection pool
	if err := db.Close(); err != nil {
		slog.Error("Error closing database connection", "error", err)
	} else {
		slog.Info("Database connection pool drained")
	}

	slog.Info("GopherWatch shutdown complete. Goodbye!")
}
