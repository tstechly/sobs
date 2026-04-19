package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	storeFactory, err := bootstrap.BuildStoreFactory()
	if err != nil {
		log.Fatal(err)
	}
	webServer := web.NewServer(cfg, storeFactory)

	receiver := otlpreceiver.NewReceiver(otlpreceiver.NewStorePipeline(storeFactory))
	grpcServer := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(grpcServer, otlpreceiver.NewTraceService(receiver))
	colmetricpb.RegisterMetricsServiceServer(grpcServer, otlpreceiver.NewMetricsService(receiver))
	collogspb.RegisterLogsServiceServer(grpcServer, otlpreceiver.NewLogsService(receiver))
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           webServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 2)

	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc serve: %w", err)
		}
	}()
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http serve: %w", err)
		}
	}()

	var serveErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		serveErr = err
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
	_ = grpcListener.Close()

	if serveErr != nil {
		log.Fatal(serveErr)
	}
}
