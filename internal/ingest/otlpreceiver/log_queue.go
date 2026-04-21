package otlpreceiver

import (
	"context"
	"log"
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
	asyncResultWait   = 15 * time.Second
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
	name string
	run  func(context.Context) error
	done chan error
}

func NewAsyncLogPipeline(base Pipeline) Pipeline {
	return &queuedPipeline{base: base}
}

func (p *queuedPipeline) ConsumeTraces(_ context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	p.ensureWorker()
	cloned, _ := proto.Clone(req).(*coltracepb.ExportTraceServiceRequest)
	return p.enqueue("traces", func(ctx context.Context) error {
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
			item.signal(nil)
			continue
		}
		err := item.run(context.Background())
		if err != nil {
			log.Printf("sobs async ingest %s failed: %v", item.name, err)
		}
		item.signal(err)
	}
}

func (p *queuedPipeline) ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error {
	p.ensureWorker()
	cloned, _ := proto.Clone(req).(*colmetricpb.ExportMetricsServiceRequest)
	return p.enqueue("metrics", func(ctx context.Context) error {
		return p.base.ConsumeMetrics(ctx, cloned)
	})
}

func (p *queuedPipeline) ConsumeLogs(_ context.Context, req *collogspb.ExportLogsServiceRequest) error {
	p.ensureWorker()
	cloned, _ := proto.Clone(req).(*collogspb.ExportLogsServiceRequest)
	return p.enqueue("logs", func(ctx context.Context) error {
		return p.base.ConsumeLogs(ctx, cloned)
	})
}

func (p *queuedPipeline) ConsumeRUM(_ context.Context, req *RUMIngestRequest) error {
	p.ensureWorker()
	cloned := cloneRUMIngestRequest(req)
	return p.enqueue("rum", func(ctx context.Context) error {
		if consumer, ok := p.base.(RUMConsumer); ok {
			return consumer.ConsumeRUM(ctx, cloned)
		}
		return nil
	})
}

func (p *queuedPipeline) ConsumeAI(_ context.Context, req *AIIngestRequest) error {
	p.ensureWorker()
	cloned := cloneAIIngestRequest(req)
	return p.enqueue("ai", func(ctx context.Context) error {
		if consumer, ok := p.base.(AIConsumer); ok {
			return consumer.ConsumeAI(ctx, cloned)
		}
		return nil
	})
}

func (p *queuedPipeline) ConsumeErrorsV1(_ context.Context, req *ErrorIngestRequest) error {
	p.ensureWorker()
	cloned := cloneErrorIngestRequest(req)
	return p.enqueue("errors", func(ctx context.Context) error {
		if consumer, ok := p.base.(ErrorConsumer); ok {
			return consumer.ConsumeErrorsV1(ctx, cloned)
		}
		return nil
	})
}

func (p *queuedPipeline) enqueue(name string, run func(context.Context) error) error {
	queued := queuedRequest{name: name, run: run}
	if asyncWaitForResultEnabled() {
		queued.done = make(chan error, 1)
	}
	timer := time.NewTimer(maxEnqueueWait)
	defer timer.Stop()
	select {
	case p.queue <- queued:
		if queued.done == nil {
			return nil
		}
		resultTimer := time.NewTimer(asyncResultWait)
		defer resultTimer.Stop()
		select {
		case err := <-queued.done:
			return err
		case <-resultTimer.C:
			return context.DeadlineExceeded
		}
	case <-timer.C:
		return WriteQueueFullError{}
	}
}

func (r queuedRequest) signal(err error) {
	if r.done == nil {
		return
	}
	select {
	case r.done <- err:
	default:
	}
	close(r.done)
}

func asyncWaitForResultEnabled() bool {
	raw := os.Getenv("SOBS_INGEST_WAIT_FOR_RESULT")
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return enabled
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
