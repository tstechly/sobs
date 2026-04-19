package main

import (
	"log"
	"net"
	"net/http"

	"github.com/abartrim/sobs/internal/bootstrap"
	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/ingest/otlpreceiver"
	"github.com/abartrim/sobs/internal/web"
	"google.golang.org/grpc"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func main() {
	cfg := config.Load()
	authProvider, err := bootstrap.BuildAuthProvider()
	if err != nil {
		log.Fatal(err)
	}
	storeFactory, err := bootstrap.BuildStoreFactory()
	if err != nil {
		log.Fatal(err)
	}
	httpServer := web.NewServer(cfg, authProvider, storeFactory)

	receiver := otlpreceiver.NewReceiver(otlpreceiver.NewNoopPipeline())
	grpcServer := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(grpcServer, otlpreceiver.NewTraceService(receiver))
	colmetricpb.RegisterMetricsServiceServer(grpcServer, otlpreceiver.NewMetricsService(receiver))
	collogspb.RegisterLogsServiceServer(grpcServer, otlpreceiver.NewLogsService(receiver))

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatal(err)
		}
	}()

	if err := http.ListenAndServe(cfg.HTTPAddr, httpServer.Handler()); err != nil {
		log.Fatal(err)
	}
}
