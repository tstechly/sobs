package otlpreceiver

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

var (
	writeQueueMax     = envInt("SOBS_WRITE_QUEUE_MAX", 5000)
	writeBatchMax     = envInt("SOBS_WRITE_BATCH_MAX", 200)
	writeBatchWait    = time.Duration(envInt("SOBS_WRITE_BATCH_WAIT_MS", 20)) * time.Millisecond
	sseQueueMax       = envInt("SOBS_SSE_QUEUE_MAX", 200)
	maxEnqueueWait    = time.Second
	keepaliveInterval = 15 * time.Second
)

type WriteQueueFullError struct{}

func (WriteQueueFullError) Error() string {
	return "write queue is full"
}

type queuedPipeline struct {
	base  Pipeline
	once  sync.Once
	queue chan queuedRequest
}

type queuedRequest struct {
	run func(context.Context) error
}

func NewAsyncLogPipeline(base Pipeline) Pipeline {
	return &queuedPipeline{base: base}
}

func (p *queuedPipeline) ConsumeTraces(_ context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	p.ensureWorker()
	cloned, _ := proto.Clone(req).(*coltracepb.ExportTraceServiceRequest)
	return p.enqueue(func(ctx context.Context) error {
		return p.base.ConsumeTraces(ctx, cloned)
	})
}

func (p *queuedPipeline) ensureWorker() {
	p.once.Do(func() {
		p.queue = make(chan queuedRequest, max(1, writeQueueMax))
		go p.worker()
	})
}

func (p *queuedPipeline) worker() {
	for {
		first := <-p.queue
		batch := []queuedRequest{first}
		deadline := time.NewTimer(writeBatchWait)
		for len(batch) < max(1, writeBatchMax) {
			select {
			case queued := <-p.queue:
				batch = append(batch, queued)
			case <-deadline.C:
				p.runBatch(batch)
				goto next
			}
		}
		if !deadline.Stop() {
			select {
			case <-deadline.C:
			default:
			}
		}
		p.runBatch(batch)
		continue
	next:
	}
}

func (p *queuedPipeline) runBatch(batch []queuedRequest) {
	for _, item := range batch {
		if item.run == nil {
			continue
		}
		_ = item.run(context.Background())
	}
}

func (p *queuedPipeline) ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error {
	p.ensureWorker()
	cloned, _ := proto.Clone(req).(*colmetricpb.ExportMetricsServiceRequest)
	return p.enqueue(func(ctx context.Context) error {
		return p.base.ConsumeMetrics(ctx, cloned)
	})
}

func (p *queuedPipeline) ConsumeLogs(_ context.Context, req *collogspb.ExportLogsServiceRequest) error {
	p.ensureWorker()
	cloned, _ := proto.Clone(req).(*collogspb.ExportLogsServiceRequest)
	return p.enqueue(func(ctx context.Context) error {
		return p.base.ConsumeLogs(ctx, cloned)
	})
}

func (p *queuedPipeline) ConsumeRUM(_ context.Context, req *RUMIngestRequest) error {
	p.ensureWorker()
	cloned := cloneRUMIngestRequest(req)
	return p.enqueue(func(ctx context.Context) error {
		if consumer, ok := p.base.(RUMConsumer); ok {
			return consumer.ConsumeRUM(ctx, cloned)
		}
		return nil
	})
}

func (p *queuedPipeline) ConsumeAI(_ context.Context, req *AIIngestRequest) error {
	p.ensureWorker()
	cloned := cloneAIIngestRequest(req)
	return p.enqueue(func(ctx context.Context) error {
		if consumer, ok := p.base.(AIConsumer); ok {
			return consumer.ConsumeAI(ctx, cloned)
		}
		return nil
	})
}

func (p *queuedPipeline) ConsumeErrorsV1(_ context.Context, req *ErrorIngestRequest) error {
	p.ensureWorker()
	cloned := cloneErrorIngestRequest(req)
	return p.enqueue(func(ctx context.Context) error {
		if consumer, ok := p.base.(ErrorConsumer); ok {
			return consumer.ConsumeErrorsV1(ctx, cloned)
		}
		return nil
	})
}

func (p *queuedPipeline) enqueue(run func(context.Context) error) error {
	queued := queuedRequest{run: run}
	timer := time.NewTimer(maxEnqueueWait)
	defer timer.Stop()
	select {
	case p.queue <- queued:
		return nil
	case <-timer.C:
		return WriteQueueFullError{}
	}
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if parsed < 1 {
		return 1
	}
	return parsed
}

func EnvSSEQueueMax() int {
	return sseQueueMax
}

func KeepaliveInterval() time.Duration {
	return keepaliveInterval
}
