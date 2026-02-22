package main

import (
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// StreamAuthInterceptor logs the Service-ID from incoming streams
func StreamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// Extract metadata from the stream's context
	md, ok := metadata.FromIncomingContext(ss.Context())

	serviceID := "unknown-service"
	if ok {
		if ids := md.Get("service-id"); len(ids) > 0 {
			serviceID = ids[0]
		}
	}

	slog.Info("Incoming stream connection", "service_id", serviceID, "method", info.FullMethod)

	// Pass execution to the actual RPC handler
	return handler(srv, ss)
}
