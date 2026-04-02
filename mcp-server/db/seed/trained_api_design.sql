-- Seed: trained API design patterns API-1 through API-10
-- Source: Thanos, OpenTelemetry Go SDK, Crossplane, gRPC-Go

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-1',
    'Multiple RPC Methods for Heterogeneous Responses: defining separate streaming RPCs for data, warnings, and metadata requires complex client orchestration. Thanos uses a union response type wrapping all response variants in a single stream.',
    'service StoreAPI {
    rpc Series(SeriesRequest) returns (stream SeriesResponse);
    rpc Warnings(SeriesRequest) returns (stream WarningResponse);
    rpc Hints(SeriesRequest) returns (stream HintResponse);
    // Client must coordinate three streams
}',
    'message SeriesResponse {
    oneof result {
        Series series = 1;
        string warning = 2;
        SeriesBatch batch = 3;
        google.protobuf.Any hints = 4;
    }
}
// Single stream carries all response types — simpler client, atomic error handling',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-2',
    'Rigid Metadata Schema in APIs: adding new metadata fields requires schema changes and version bumps. Thanos uses google.protobuf.Any for extensible hints that stores can pass without schema changes.',
    'message QueryResponse {
    repeated Series series = 1;
    QueryHints hints = 2; // adding a field requires proto change + rebuild
}',
    'message QueryResponse {
    repeated Series series = 1;
    google.protobuf.Any hints = 2; // extensible without schema change
}
// Producers define their own hint types; consumers use type assertions',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-3',
    'Adding Methods to Stable Interfaces: adding a method to an interface with external implementations breaks all consumers. OTel Go mandates companion interfaces discovered via type assertion for new capabilities.',
    'type Exporter interface {
    Export(ctx context.Context, spans []Span) error
    Flush() error // BREAKS: every existing Exporter implementation
}',
    'type Exporter interface {
    Export(ctx context.Context, spans []Span) error
    // Never add methods to stable interfaces
}

// New capability via companion interface
type Flusher interface {
    Flush() error
}

// Discovery via type assertion
if f, ok := exporter.(Flusher); ok {
    f.Flush()
}',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-4',
    'Implicit Type Conversion at Boundaries: passing protobuf types directly into domain logic or vice versa couples internal code to wire format. Thanos uses explicit bidirectional conversion functions at boundaries.',
    'func queryStore(matchers []*labels.Matcher) (*storepb.SeriesResponse, error) {
    // Passing domain types to proto layer — coupling
    resp, err := client.Series(ctx, &storepb.SeriesRequest{Matchers: matchers})
}',
    'func queryStore(matchers []*labels.Matcher) (*storepb.SeriesResponse, error) {
    protoMatchers := PromMatchersToMatchers(matchers) // explicit conversion
    resp, err := client.Series(ctx, &storepb.SeriesRequest{Matchers: protoMatchers})
    if err != nil {
        return nil, err
    }
    // Convert back at consumption boundary
    series := MatchersToPromMatchers(resp.Matchers)
}',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-5',
    'Stringly-Typed Component Classification: using string constants to classify components is error-prone and lacks compile-time safety. Thanos uses marker interfaces with empty methods as compile-time type assertions.',
    'const (
    ComponentStore  = "store"
    ComponentQuery  = "query"
    ComponentCompact = "compact"
)

func isStore(component string) bool {
    return component == ComponentStore // no compile-time safety
}',
    'type Component interface { String() string }
type StoreAPI interface { Component; implementsStoreAPI() }
type Source interface { Component; producesBlocks() }

// Compile-time verification
var _ StoreAPI = Store
var _ Source = Compactor

// Type-safe classification
func isStore(c Component) bool {
    _, ok := c.(StoreAPI)
    return ok
}',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-6',
    'All-or-Nothing Failure in Distributed Queries: aborting a multi-source query on the first error prevents returning partial results. Thanos supports per-request partial response strategy — queries can choose between strict and best-effort modes.',
    'func queryAll(ctx context.Context, stores []Store, req *Request) (*Result, error) {
    for _, store := range stores {
        data, err := store.Query(ctx, req)
        if err != nil {
            return nil, err // one failed store aborts everything
        }
        result.Merge(data)
    }
    return result, nil
}',
    'func queryAll(ctx context.Context, stores []Store, req *Request) (*Result, error) {
    var warnings []string
    for _, store := range stores {
        data, err := store.Query(ctx, req)
        if err != nil {
            if req.PartialResponseStrategy == storepb.PartialResponseStrategy_WARN {
                warnings = append(warnings, err.Error())
                continue // best-effort: skip failed store
            }
            return nil, err // strict: abort on first error
        }
        result.Merge(data)
    }
    result.Warnings = warnings
    return result, nil
}',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-7',
    'Constructor With Positional Parameters: functions with many positional parameters are error-prone and hard to extend. OTel Go standardizes functional options with With*/Without* naming conventions.',
    'func NewTracer(name string, version string, schemaURL string, sampler Sampler, exporter Exporter) *Tracer {
    // Adding a parameter breaks all callers
}',
    'type Option interface {
    apply(config) config
}

func NewTracer(name string, opts ...Option) *Tracer {
    cfg := newConfig(opts...)
    // ...
}

// Naming convention:
// Enable:  WithSampler(s)
// Disable: WithoutSampling()
// Typed:   WithVersion(v string)',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-8',
    'Lock-Based Global Provider Access: acquiring a mutex on every span creation or metric recording adds overhead in the hot path. OTel Go uses atomic.Value for lock-free reads with sync.Once for one-time delegation.',
    'var (
    mu       sync.Mutex
    provider TracerProvider
)

func GetTracerProvider() TracerProvider {
    mu.Lock()
    defer mu.Unlock()
    return provider // lock on every read — contention in hot path
}',
    'var globalProvider atomic.Value

func GetTracerProvider() TracerProvider {
    if p, ok := globalProvider.Load().(TracerProvider); ok {
        return p // lock-free read in hot path
    }
    return noopProvider
}

var delegateOnce sync.Once

func SetTracerProvider(p TracerProvider) {
    delegateOnce.Do(func() {
        // Delegate pre-initialization consumers
    })
    globalProvider.Store(p)
}',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-9',
    'Lock-Based Processor Registration on Read Path: acquiring a write lock to read processor lists blocks the hot path during registration. OTel Go uses copy-on-write with atomic pointer — reads are lock-free, writes copy the slice.',
    'type TracerProvider struct {
    mu         sync.Mutex
    processors []SpanProcessor
}

func (tp *TracerProvider) getProcessors() []SpanProcessor {
    tp.mu.Lock()         // blocks during registration
    defer tp.mu.Unlock()
    return tp.processors
}',
    'type TracerProvider struct {
    mu         sync.Mutex
    processors atomic.Pointer[[]SpanProcessor]
}

func (tp *TracerProvider) getProcessors() []SpanProcessor {
    if p := tp.processors.Load(); p != nil {
        return *p // lock-free read
    }
    return nil
}

func (tp *TracerProvider) RegisterSpanProcessor(sp SpanProcessor) {
    tp.mu.Lock()
    defer tp.mu.Unlock()
    old := tp.getProcessors()
    // Copy-on-write: new slice, append, atomic store
    newProcs := make([]SpanProcessor, len(old)+1)
    copy(newProcs, old)
    newProcs[len(old)] = sp
    tp.processors.Store(&newProcs)
}',
    'trained',
    'api-design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'API-10',
    'Mutable Resource After Construction: resources that can be modified after creation require synchronization and create race conditions. OTel Go makes resources immutable after construction with merge semantics for combining.',
    'type Resource struct {
    mu    sync.Mutex
    attrs map[string]string
}

func (r *Resource) Set(key, value string) {
    r.mu.Lock()
    r.attrs[key] = value // mutable — race-prone, hard to reason about
    r.mu.Unlock()
}',
    'type Resource struct {
    attrs attribute.Set // immutable after construction
    schemaURL string
}

func NewResource(opts ...ResourceOption) (*Resource, error) {
    // Fully constructed and immutable after this call
    return &Resource{attrs: buildAttrs(opts...), schemaURL: schema}, nil
}

// Combining resources: merge semantics (b overwrites a for conflicts)
func Merge(a, b *Resource) (*Resource, error) {
    if a.schemaURL != b.schemaURL && a.schemaURL != "" && b.schemaURL != "" {
        return merged, fmt.Errorf("schema URL conflict: %s vs %s", a.schemaURL, b.schemaURL)
    }
    return merged, nil
}',
    'trained',
    'api-design'
);
