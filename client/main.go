package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	pb "gopherwatch/proto" // Adjust to your actual module path

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	numAgents   = 150 // Simulating 100+ agents
	reportsEach = 15  // How many reports each agent sends
)

func main() {
	// 1. Open a single connection to the server
	// gRPC multiplexes multiple streams over this single HTTP/2 connection
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewMetricsServiceClient(conn)

	// We use a WaitGroup to wait for all agents to finish streaming
	var wg sync.WaitGroup

	log.Printf("Starting %d concurrent agents...", numAgents)

	// 2. Spin up goroutines
	for i := 1; i <= numAgents; i++ {
		wg.Add(1)
		go simulateAgent(i, client, &wg)
	}

	// 3. Wait for all agents to complete
	wg.Wait()
	log.Println("All agents have successfully finished streaming.")
}

// simulateAgent represents a single microservice streaming data
func simulateAgent(id int, client pb.MetricsServiceClient, wg *sync.WaitGroup) {
	defer wg.Done()

	serviceID := fmt.Sprintf("service-agent-%03d", id)
	md := metadata.Pairs("service-id", serviceID)
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	// --- NEW: UNARY CALL ---
	// Agent registers itself before starting the stream
	agentInfo := &pb.AgentInfo{
		OsVersion:    "Linux 5.15",
		AgentVersion: "v1.2.0",
	}

	config, err := client.RegisterAgent(ctx, agentInfo)
	if err != nil {
		log.Printf("[%s] Registration failed: %v", serviceID, err)
		return
	}

	// Only print for a few agents so we don't spam the console too much
	if id%20 == 0 {
		log.Printf("[%s] Registered successfully. Server config: interval=%dms",
			serviceID, config.ReportIntervalMs)
	}
	// -----------------------

	// Open the client-to-server stream
	stream, err := client.SendMetrics(ctx)
	if err != nil {
		log.Printf("[%s] Error opening stream: %v", serviceID, err)
		return
	}

	for i := 0; i < reportsEach; i++ {
		cpu := rand.Float64() * 100.0

		report := &pb.MetricReport{
			CpuUsage:     cpu,
			MemoryUsage:  rand.Float64() * 16000.0,
			RequestCount: int32(rand.Intn(500)),
			Timestamp:    time.Now().Format(time.RFC3339),
		}

		if err := stream.Send(report); err != nil {
			log.Printf("[%s] Failed to send metric: %v", serviceID, err)
			return
		}

		// Use the interval provided by the server's unary response!
		time.Sleep(time.Duration(config.ReportIntervalMs) * time.Millisecond)
	}

	summary, err := stream.CloseAndRecv()
	if err != nil {
		log.Printf("[%s] Error receiving summary: %v", serviceID, err)
		return
	}

	if id%20 == 0 {
		log.Printf("[%s] Stream closed cleanly. Summary Status: %s", serviceID, summary.Status)
	}
}
