-- Seed: trained mesh/proxy patterns MESH-1 through MESH-16
-- Source: Traefik, Linkerd2, Istio, gRPC-Go

-- === Middleware & Interceptor Patterns ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-1',
    'Rigid Middleware Ordering: hard-wiring middleware order in application code makes chains untestable and non-configurable. Traefik uses composable chain construction (alice) with uniform constructor signatures.',
    'func buildChain(next http.Handler) http.Handler {
    return rateLimit(auth(compress(next))) // rigid, untestable
}',
    'type middlewareChainBuilder interface {
    BuildMiddlewareChain(ctx context.Context, middlewares []string) *alice.Chain
}

// Every middleware follows uniform signature: (ctx, next, config, name) -> (handler, error)
func New(ctx context.Context, next http.Handler,
    config dynamic.RateLimit, name string) (http.Handler, error) {
    // ...
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-2',
    'Interceptors as Interfaces: defining interceptors as interfaces with a single method adds ceremony without benefit. gRPC-Go uses function types with the "next" handler passed as a parameter, enabling natural closure composition.',
    'type Interceptor interface {
    Intercept(ctx context.Context, req any, next Handler) (any, error)
}',
    '// Function types, not interfaces — natural for chaining
type UnaryServerInterceptor func(ctx context.Context, req any,
    info *UnaryServerInfo, handler UnaryHandler) (resp any, err error)

// Chain via recursive closures with index
func chainUnaryInterceptors(interceptors []UnaryServerInterceptor) UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
        return interceptors[0](ctx, req, info,
            getChainHandler(interceptors, 0, info, handler))
    }
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-3',
    'No Recursion Detection in Middleware Chains: middleware configurations that reference each other create infinite loops. Traefik checks for circular references before building chains.',
    'func buildMiddleware(ctx context.Context, name string) (http.Handler, error) {
    config := getConfig(name)
    // No check — if config references itself, infinite recursion
    return buildChain(ctx, config.Middlewares)
}',
    'func buildMiddleware(ctx context.Context, name string) (http.Handler, error) {
    if err := recursion.CheckRecursion(ctx, name); err != nil {
        return nil, fmt.Errorf("circular middleware reference: %w", err)
    }
    config := getConfig(name)
    return buildChain(ctx, config.Middlewares)
}',
    'trained',
    'mesh-proxy'
);

-- === Dynamic Configuration & Push Patterns ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-4',
    'Polling-Based Config Provider: polling for config changes wastes resources and adds latency. Traefik uses channel-based providers that push updates. Istio uses xDS streaming.',
    'type Provider interface {
    GetConfig() (*Config, error) // caller must poll
}

for {
    cfg, _ := provider.GetConfig()
    apply(cfg)
    time.Sleep(30 * time.Second)
}',
    'type Provider interface {
    Provide(configChan chan<- dynamic.Message, pool *safe.Pool) error
    Init() error
}

// Provider pushes updates through channel — no polling
func (p *KubeProvider) Provide(configChan chan<- dynamic.Message, pool *safe.Pool) error {
    // Watch K8s resources, push on change
    configChan <- dynamic.Message{ProviderName: p.name, Configuration: conf}
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-5',
    'Emitting Config on Every Event: pushing config downstream on every K8s event floods consumers with duplicates. Traefik hashes the entire config and only emits when the hash changes.',
    'case evt := <-watcher.Events:
    conf := loadConfig()
    configChan <- conf // floods downstream with duplicates',
    'case evt := <-watcher.Events:
    conf := loadConfig()
    hash, err := hashstructure.Hash(conf, nil)
    if err != nil || hash == p.lastConfigHash {
        continue // no actual change
    }
    p.lastConfigHash = hash
    configChan <- dynamic.Message{Configuration: conf}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-6',
    'No Push Debouncing: pushing config to every proxy on each change event causes thundering herd during rolling deployments. Istio uses a 3-phase pipeline: event channel -> debounce (quiet period + max delay) -> push queue with per-connection merging.',
    'func onConfigChange(event ConfigEvent) {
    for _, proxy := range allProxies {
        proxy.Push(generateConfig()) // N events x M proxies = thundering herd
    }
}',
    'type DiscoveryServer struct {
    pushChannel chan *PushRequest // receives raw events
    pushQueue   *PushQueue       // per-connection merging
}

func (s *DiscoveryServer) debounce(ch chan *PushRequest, opts DebounceOptions) {
    var pending *PushRequest
    var timer <-chan time.Time
    var start time.Time
    for {
        select {
        case req := <-ch:
            pending = pending.Merge(req) // coalesce events
            if timer == nil {
                start = time.Now()
                timer = time.After(opts.DebounceAfter)
            }
        case <-timer:
            if time.Since(start) >= opts.DebounceMax {
                s.Push(pending) // force push after max delay
                pending = nil
                timer = nil
            } else {
                timer = time.After(opts.DebounceAfter) // reset quiet timer
            }
        }
    }
}',
    'trained',
    'mesh-proxy'
);

-- === Connection & Handler Management ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-7',
    'Server Restart for Config Reload: stopping and restarting the server to apply new configuration drops all connections. Traefik uses RWMutex-based handler hot-swap — new requests get the updated handler while in-flight requests complete on their snapshot.',
    'func reloadConfig(srv *http.Server, newHandler http.Handler) {
    srv.Close()          // drops all connections!
    srv.Handler = newHandler
    srv.ListenAndServe()
}',
    'type HandlerSwitcher struct {
    handler safe.Safe // RWMutex-protected value
}

func (h *HandlerSwitcher) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
    handler := h.handler.Get().(http.Handler) // RLock — in-flight uses snapshot
    handler.ServeHTTP(rw, req)
}

func (h *HandlerSwitcher) UpdateHandler(newHandler http.Handler) {
    h.handler.Set(newHandler) // WLock — new requests get updated handler
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-8',
    'Goroutine-Per-Connection Push: spawning a goroutine for each proxy connection on every config change does not scale. Istio uses a fixed worker pool pulling from a push queue with per-connection request merging.',
    'func pushToAll(proxies []*Connection, update *Config) {
    for _, p := range proxies {
        go p.Push(update) // 10k connections = 10k goroutines per change
    }
}',
    'func (s *Server) sendPushes(stopCh <-chan struct{}) {
    for i := 0; i < concurrentPushLimit; i++ {
        go func() {
            for {
                con, req := s.pushQueue.Dequeue() // blocks until work
                if con == nil { return }
                s.pushConnection(con, req)
                s.pushQueue.MarkDone(con) // may re-enqueue if new updates arrived
            }
        }()
    }
}',
    'trained',
    'mesh-proxy'
);

-- === gRPC Streaming & Service Mesh Patterns ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-9',
    'Blocking in Informer Callbacks: blocking in K8s informer event handlers while waiting for a gRPC stream consumer causes deadlocks. Linkerd uses buffered channels with non-blocking enqueue and stream abort on overflow.',
    'func (et *translator) onEndpointUpdate(obj interface{}) {
    et.stream.Send(translate(obj)) // blocks if consumer is slow — deadlock risk
}',
    'func (et *translator) onEndpointUpdate(update interface{}) {
    select {
    case et.updates <- update: // non-blocking enqueue
    default:
        et.overflowCounter.Inc()
        select {
        case <-et.endStream: // already closing
        default:
            et.log.Error("update queue full; aborting stream")
            close(et.endStream) // signal stream to close
        }
    }
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-10',
    'Calling gRPC Stream.Send from Multiple Goroutines: gRPC streams are not safe for concurrent Send calls. Linkerd wraps streams with a synchronized channel-based sender.',
    'func (t *translator) sendUpdate(update *pb.Update) {
    t.stream.Send(update) // NOT thread-safe if called from multiple goroutines
}',
    'type synchronizedStream struct {
    done  chan struct{}
    ch    chan *pb.Update // unbuffered — synchronizes sends
    inner pb.Destination_GetServer
}

func (s *synchronizedStream) Send(update *pb.Update) error {
    select {
    case s.ch <- update: return nil // serialized through channel
    case <-s.done: return errStreamStopped
    }
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-11',
    'Monolithic Update Handler: processing all types of updates in a single handler becomes unwieldy. Linkerd composes small listener stages into processing pipelines, built backwards from stream to source.',
    'func handleUpdate(update interface{}) {
    deduplicate(update)
    mergeOpaquePorts(update)
    applyDefaults(update)
    sendToStream(update) // monolithic, hard to test individual stages
}',
    '// Compose pipeline backwards: stream <- translator <- opaque <- dedup <- default <- source
translator := newProfileTranslator(stream)
opaqueAdaptor := newOpaquePortsAdaptor(translator)
dedup := newDedupProfileListener(opaqueAdaptor)
defaultProfile := newDefaultProfileListener(dedup)

s.profiles.Subscribe(profileID, defaultProfile) // source feeds the pipeline',
    'trained',
    'mesh-proxy'
);

-- === mTLS & Identity Patterns ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-12',
    'Combined Auth Step: combining authentication and authorization in a single step makes it impossible to distinguish "who are you?" from "what can you do?" failures. Istio and Linkerd use layered pipelines: authenticate -> authorize -> sign.',
    'func issueCert(ctx context.Context, csr []byte) ([]byte, error) {
    if !isAuthorized(ctx) {
        return nil, errors.New("unauthorized") // which step failed?
    }
    return sign(csr)
}',
    'func (s *Server) issueCert(ctx context.Context, req *CertifyRequest) (*CertifyResponse, error) {
    // 1. Authenticate (who are you?)
    caller, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }
    // 2. Authorize (what can you do?)
    if err := s.authorize(caller, req); err != nil {
        return nil, status.Error(codes.PermissionDenied, err.Error())
    }
    // 3. Sign
    cert, err := s.ca.Sign(req.CSR, certOpts)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    return &CertifyResponse{CertChain: cert}, nil
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-13',
    'Trust Domain as Raw String: passing trust domains as raw strings throughout the codebase is error-prone and lacks validation. Linkerd wraps trust domain in a validated type with constructor validation.',
    'func getIdentity(ns, sa, domain string) string {
    return fmt.Sprintf("spiffe://%s/ns/%s/sa/%s", domain, ns, sa) // no validation
}',
    'type TrustDomain struct {
    controlNS, domain string
}

func NewTrustDomain(controlNS, domain string) (*TrustDomain, error) {
    if errs := validation.IsDNS1123Label(controlNS); len(errs) > 0 {
        return nil, fmt.Errorf("invalid label %q: %s", controlNS, errs[0])
    }
    return &TrustDomain{controlNS: controlNS, domain: domain}, nil
}

func (d *TrustDomain) Identity(typ, name, ns string) string {
    return fmt.Sprintf("%s.%s.serviceaccount.identity.%s.%s", name, ns, d.controlNS, d.domain)
}',
    'trained',
    'mesh-proxy'
);

-- === Load Balancing & Health Check Patterns ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-14',
    'Blocking Load Balancer Pick: a load balancer Pick function that blocks while waiting for a healthy backend adds latency to every request. gRPC-Go mandates non-blocking pickers that return ErrNoSubConnAvailable to be retried after state change.',
    'func (b *Balancer) Pick(info PickInfo) (*Backend, error) {
    b.mu.Lock()
    defer b.mu.Unlock()
    for !b.hasReady() {
        b.cond.Wait() // blocks request until backend ready
    }
    return b.next(), nil
}',
    'type Picker interface {
    Pick(info PickInfo) (PickResult, error) // MUST NOT block
}

func (p *roundRobinPicker) Pick(info PickInfo) (PickResult, error) {
    if len(p.subConns) == 0 {
        return PickResult{}, balancer.ErrNoSubConnAvailable // retried on state change
    }
    sc := p.subConns[p.next%len(p.subConns)]
    p.next++
    return PickResult{
        SubConn: sc,
        Done: func(di DoneInfo) { /* completion callback */ },
    }, nil
}',
    'trained',
    'mesh-proxy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-15',
    'Only Active Health Checking: relying solely on periodic probes misses transient failures between probe intervals. Traefik combines active probing with passive failure tracking in a sliding window.',
    'func healthCheck(targets []string) {
    for _, t := range targets {
        if !probe(t) { markUnhealthy(t) }
    }
    // Misses failures between probes
}',
    '// Active: periodic HTTP/gRPC probing
go func() {
    ticker := time.NewTicker(interval)
    for range ticker.C {
        for _, t := range targets {
            if !probe(t) { markUnhealthy(t) }
        }
    }
}()

// Passive: sliding window failure tracking on real traffic
func (p *PassiveHealthChecker) RecordFailure(target string) {
    p.pruneOldFailures(target) // remove failures outside window
    p.failures[target] = append(p.failures[target], time.Now())
    if len(p.failures[target]) >= p.maxFailed {
        p.balancer.SetStatus(target, false) // immediate detection
    }
}',
    'trained',
    'mesh-proxy'
);

-- === Testing Patterns ===

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'MESH-16',
    'Real TCP Connections in gRPC Unit Tests: using real TCP listeners in gRPC tests requires port allocation, is flaky, and slow. gRPC-Go provides bufconn — in-memory pipe pairs implementing net.Listener with full net.Conn semantics.',
    'func TestMyService(t *testing.T) {
    lis, _ := net.Listen("tcp", ":0") // real port allocation, flaky
    srv := grpc.NewServer()
    pb.RegisterMyServiceServer(srv, &myImpl{})
    go srv.Serve(lis)
    conn, _ := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
}',
    'func TestMyService(t *testing.T) {
    lis := bufconn.Listen(1024 * 1024) // in-memory, no ports
    srv := grpc.NewServer()
    pb.RegisterMyServiceServer(srv, &myImpl{})
    go srv.Serve(lis)

    conn, _ := grpc.NewClient("passthrough:///bufconn",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    client := pb.NewMyServiceClient(conn)
    // test with client...
}',
    'trained',
    'mesh-proxy'
);
