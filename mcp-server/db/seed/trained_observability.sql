-- Seed: trained observability patterns OBS-1 through OBS-10
-- Source: Thanos, OpenTelemetry Go SDK

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-1',
    'Single Metric for Scan vs Fetch: using one metric for both index scanning and data retrieval makes it impossible to diagnose whether slowness comes from scanning or loading. Thanos uses touched-vs-fetched metric pairs.',
    'var queryDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{Name: "query_duration_seconds"},
    []string{"type"},
)
// Cannot distinguish scan time from fetch time',
    'var (
    postingsTouched = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name: "postings_touched_total",
        Help: "Number of postings scanned during query",
    })
    postingsFetched = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name: "postings_fetched_total",
        Help: "Number of postings actually loaded from storage",
    })
)
// Separate metrics reveal: high touched + low fetched = inefficient index scan',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-2',
    'Single Duration Metric for Parallel Work: using one timer for operations with parallelism hides whether latency comes from concurrency limits or computation. Thanos tracks concurrent work time vs wall-clock elapsed separately.',
    'var opDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
    Name: "operation_duration_seconds",
})
// Cannot tell if 10s latency is 10s of work or 1s of work waiting 9s for a semaphore',
    'var (
    opWorkDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name: "operation_work_duration_seconds",
        Help: "Time spent doing actual work (sum of goroutine CPU time)",
    })
    opWallDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name: "operation_wall_duration_seconds",
        Help: "Wall-clock elapsed time including concurrency waits",
    })
)
// wall >> work means concurrency bottleneck; wall ≈ work means compute-bound',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-3',
    'No Progress Metrics for Long Operations: long-running operations (compaction, migration, sync) without progress metrics leave operators blind to stalls. Thanos exposes predicted remaining work as Prometheus gauges.',
    'func compact(ctx context.Context, blocks []Block) error {
    for _, b := range blocks {
        if err := compactBlock(ctx, b); err != nil {
            return err
        }
    }
    return nil // no way to know progress from outside
}',
    'var (
    compactTodo = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "compact_todo_blocks",
        Help: "Number of blocks remaining to compact",
    })
    compactDone = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "compact_done_blocks",
        Help: "Number of blocks compacted so far",
    })
)

func compact(ctx context.Context, blocks []Block) error {
    compactTodo.Set(float64(len(blocks)))
    for i, b := range blocks {
        if err := compactBlock(ctx, b); err != nil {
            return err
        }
        compactDone.Set(float64(i + 1))
        compactTodo.Set(float64(len(blocks) - i - 1))
    }
    return nil
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-4',
    'No Debug Tracing Override: requiring sampling config changes to trace a single request in production is slow and risky. Thanos supports X-Thanos-Force-Tracing header to force sampling per-request.',
    'func tracingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Sampling decision is global — cannot trace a specific request
        ctx, span := tracer.Start(r.Context(), r.URL.Path)
        defer span.End()
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}',
    'func tracingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        opts := []trace.SpanStartOption{}
        if r.Header.Get("X-Force-Tracing") != "" {
            opts = append(opts, trace.WithAttributes(
                attribute.Bool("force_sampled", true),
            ))
            // Force this request to be sampled regardless of global config
        }
        ctx, span := tracer.Start(r.Context(), r.URL.Path, opts...)
        defer span.End()
        // Return trace ID in response for correlation
        w.Header().Set("X-Trace-Id", span.SpanContext().TraceID().String())
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-5',
    'Missing Trace ID in Error Responses: returning errors without trace IDs makes it impossible to correlate user-visible errors with distributed traces. Thanos returns X-Thanos-Trace-Id in all responses.',
    'func handleError(w http.ResponseWriter, err error) {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    // User sees error but cannot provide trace ID for debugging
}',
    'func handleError(w http.ResponseWriter, r *http.Request, err error) {
    span := trace.SpanFromContext(r.Context())
    traceID := span.SpanContext().TraceID().String()
    w.Header().Set("X-Trace-Id", traceID)
    http.Error(w, fmt.Sprintf("error (trace: %s): %s", traceID, err), http.StatusInternalServerError)
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-6',
    'Uniform gRPC Logging: logging every gRPC call at the same level floods logs from high-volume RPCs. Thanos uses per-method YAML-driven log configuration to control which methods emit logs.',
    'func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    log.Info("request", "method", info.FullMethod) // floods with high-volume RPCs
    return handler(ctx, req)
}',
    'type MethodLogConfig struct {
    LogStart bool `yaml:"log_start"`
    LogEnd   bool `yaml:"log_end"`
}

func loggingInterceptor(methodConfigs map[string]MethodLogConfig) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        cfg := methodConfigs[info.FullMethod]
        if cfg.LogStart {
            log.Info("request start", "method", info.FullMethod)
        }
        resp, err := handler(ctx, req)
        if cfg.LogEnd {
            log.Info("request end", "method", info.FullMethod, "err", err)
        }
        return resp, err
    }
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-7',
    'Telemetry Errors Disrupting Application: telemetry failures (export, flush) that propagate errors to application code can break the instrumented service. OTel Go uses a global fire-and-forget ErrorHandler that never disrupts the caller.',
    'func recordMetric(name string, value float64) error {
    if err := exporter.Export(name, value); err != nil {
        return fmt.Errorf("export metric: %w", err) // breaks application on telemetry failure
    }
    return nil
}',
    'func recordMetric(name string, value float64) {
    if err := exporter.Export(name, value); err != nil {
        otel.Handle(err) // fire-and-forget: logs to stderr, never disrupts caller
    }
    // Application continues regardless of telemetry health
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-8',
    'Observability Overhead When Disabled: performing allocations or lock acquisitions for unsampled/disabled telemetry wastes CPU in production. OTel Go uses non-recording span stubs that perform no work.',
    'func (s *span) SetAttributes(attrs ...Attribute) {
    s.mu.Lock()
    s.attributes = append(s.attributes, attrs...) // allocates even if not recording
    s.mu.Unlock()
}',
    'func (s *span) SetAttributes(attrs ...Attribute) {
    if !s.isRecording() {
        return // zero overhead when not sampled
    }
    s.mu.Lock()
    s.attributes = append(s.attributes, attrs...)
    s.mu.Unlock()
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-9',
    'Swallowed Cleanup Errors in Deferred Close: using defer f.Close() silently discards close errors, which can mask data loss (unflushed writes). Thanos uses CloseWithErrCapture to merge cleanup errors into the named return value.',
    'func readData(path string) (data []byte, err error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close() // close error silently discarded
    return io.ReadAll(f)
}',
    'func readData(path string) (data []byte, err error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer runutil.CloseWithErrCapture(&err, f, "close data file")
    // Close error merged into named return — caller sees both read and close errors
    return io.ReadAll(f)
}',
    'trained',
    'observability'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OBS-10',
    'Undrained HTTP Response Body: closing an HTTP response body without draining it breaks HTTP keep-alive, forcing a new TCP connection per request. Thanos uses ExhaustCloseWithLogOnErr to drain before closing.',
    'func fetch(url string) ([]byte, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close() // breaks keep-alive if body not fully read
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("status %d", resp.StatusCode) // body not drained
    }
    return io.ReadAll(resp.Body)
}',
    'func fetch(url string) ([]byte, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer func() {
        io.Copy(io.Discard, resp.Body) // drain to preserve keep-alive
        resp.Body.Close()
    }()
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("status %d", resp.StatusCode)
    }
    return io.ReadAll(resp.Body)
}',
    'trained',
    'observability'
);
