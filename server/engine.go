package main

import (
	"database/sql"
	"log/slog"
	"sync"

	pb "gopherwatch/proto" // Replace with your actual module path
)

// 1. Define AlertRule and Evaluator
type AlertRule struct {
	Metric    string
	Threshold float64
}

type Evaluator interface {
	Evaluate(serviceID string, report *pb.MetricReport)
}

// ServiceMetric wraps the report with its ServiceID for the channel
type ServiceMetric struct {
	ServiceID string
	Report    *pb.MetricReport
}

// Engine holds the rules, the dispatcher channel, and the thread-safe state map
type Engine struct {
	rules      []AlertRule
	dispatcher chan ServiceMetric // The Fan-out channel

	mu    sync.RWMutex
	state map[string]*pb.MetricReport // The Current State
	db    *sql.DB
}

// NewEngine initializes our alerting engine
func NewEngine(rules []AlertRule, bufferSize int, db *sql.DB) *Engine {
	return &Engine{
		rules:      rules,
		dispatcher: make(chan ServiceMetric, bufferSize),
		state:      make(map[string]*pb.MetricReport),
		db:         db,
	}
}

// StartWorkers spins up our worker pool
func (e *Engine) StartWorkers(numWorkers int) {
	for i := 0; i < numWorkers; i++ {
		go e.worker(i)
	}
}

// worker pulls from the channel and evaluates rules
func (e *Engine) worker(id int) {
	for sm := range e.dispatcher {
		// Update the global state safely
		e.mu.Lock()
		e.state[sm.ServiceID] = sm.Report
		e.mu.Unlock()

		// Evaluate against rules
		e.Evaluate(sm.ServiceID, sm.Report)
	}
}

// Evaluate checks the metric against thresholds and logs alerts
func (e *Engine) Evaluate(serviceID string, report *pb.MetricReport) {
	for _, rule := range e.rules {
		if rule.Metric == "CPU" && report.CpuUsage > rule.Threshold {
			slog.Error("CRITICAL ALERT", "service", serviceID, "metric", "CPU", "value", report.CpuUsage, "threshold", rule.Threshold)
			SaveAlert(e.db, serviceID, "CPU", report.CpuUsage)
		}
		if rule.Metric == "MEMORY" && report.MemoryUsage > rule.Threshold {
			slog.Error("CRITICAL ALERT", "service", serviceID, "metric", "MEMORY", "value", report.MemoryUsage, "threshold", rule.Threshold)
			SaveAlert(e.db, serviceID, "MEMORY", report.MemoryUsage)
		}
	}
}

func (e *Engine) RemoveService(serviceID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.state, serviceID)
}
