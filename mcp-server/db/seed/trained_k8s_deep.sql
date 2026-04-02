-- Seed: trained K8s deep patterns K8S-1 through K8S-10
-- Source: cert-manager, scheduler-plugins, ArgoCD, Cilium, Calico

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-1',
    'Self-Registering Controller Registry: hard-coded controller instantiation in main() requires changes for every new controller. Module-level Constructor map with init() registration decouples controllers from wiring.',
    'func main() {
    issuerCtrl := issuers.NewController(deps...)
    certCtrl := certificates.NewController(deps...)
    orderCtrl := orders.NewController(deps...)
}',
    'var known = make(map[string]Constructor)
type Constructor func(ctx *ContextFactory) (Interface, error)
func Register(name string, fn Constructor) { known[name] = fn }
// Each controller: func init() { Register("certificates", NewCertController) }
// Main: for name, ctor := range Known() { ctrl, _ := ctor(ctx) }',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-2',
    'ContextFactory for Shared Infrastructure: each controller independently creating clients and informers wastes memory and hammers the API server. Shared ContextFactory with per-component UserAgent and EventRecorder.',
    'func NewMyController() *Controller {
    config := rest.InClusterConfig()
    client := kubernetes.NewForConfig(config)
    informer := informers.NewSharedInformerFactory(client, 0)
    // Duplicated across every controller
}',
    'type ContextFactory struct {
    baseRestConfig *rest.Config
    ctx            *Context // shared informers, metrics, clock
}
func (c *ContextFactory) Build(component ...string) (*Context, error) {
    restConfig := util.RestConfigWithUserAgent(c.baseRestConfig, component...)
    ctx := *c.ctx // shared informers
    ctx.FieldManager = util.PrefixFromUserAgent(restConfig.UserAgent)
    ctx.Recorder = broadcaster.NewRecorder(scheme, corev1.EventSource{Component: component[0]})
    return &ctx, nil
}',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-3',
    'Composable Policy Functions for Status: monolithic validation in Reconcile() with 20+ inline checks is untestable. Chain of PolicyFunc(input) returning (reason, message, violated). Clock injected via closure for deterministic tests.',
    'func (c *Controller) Reconcile(ctx context.Context, req Request) (Result, error) {
    if secret == nil {
        setCondition(cert, "Ready", "False", "DoesNotExist", "...")
        return ...
    }
    // 20+ more inline checks, deeply nested
}',
    'type PolicyFunc func(input Input) (reason, message string, violated bool)
var ReadinessPolicies = []PolicyFunc{
    SecretDoesNotExist,
    SecretPublicKeysDiffer,
    CurrentCertificateNearingExpiry(clock),
}
for _, check := range ReadinessPolicies {
    reason, message, violated := check(input)
    if violated { setCondition(cert, "Issuing", "True", reason, message); break }
}',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-4',
    'Cross-Resource Ownership Chain: complex resource relationships without ownership tracking orphan children on parent deletion. Multi-level ownerReferences with GC cascade and status propagation up the chain.',
    'func createOrder(cert *Certificate) error {
    order := &Order{Spec: OrderSpec{Request: cert.Spec.Request}}
    return client.Create(ctx, order) // no owner reference, orphaned on cert delete
}',
    'func createOrder(cert *Certificate) error {
    order := &Order{
        ObjectMeta: metav1.ObjectMeta{
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(cert, certificateGVK),
            },
        },
    }
    return client.Create(ctx, order)
    // GC cascades deletion: Certificate -> CertificateRequest -> Order -> Challenge
}',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-5',
    'Scheduler Framework Plugin Interface: forking the scheduler to add custom logic means missing upstream improvements. Implement framework extension points (PreFilter, Filter, Score, Reserve, Permit) to compose with, not replace, the default scheduler.',
    'func schedule(pod *v1.Pod) (string, error) {
    // Forked scheduler with custom logic
    for _, node := range nodes {
        if customFilter(pod, node) { return node.Name, nil }
    }
    return "", errors.New("no node")
}',
    'type CoschedulingPlugin struct{}
func (p *CoschedulingPlugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
    pg := p.getPodGroup(pod)
    if pg == nil { return nil, framework.NewStatus(framework.Success) }
    if !p.minMemberReady(pg) { return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable) }
    return nil, framework.NewStatus(framework.Success)
}',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-6',
    'Gang Scheduling with PodGroup: submitting batch jobs without atomicity guarantees causes partial scheduling, resource waste, and deadlocks. PodGroup CRD with minMember + Permit gate ensures all-or-nothing scheduling.',
    'func submitBatch(pods []*v1.Pod) {
    for _, pod := range pods {
        client.Create(ctx, pod) // some may schedule, others stuck
    }
}',
    'pg := &PodGroup{Spec: PodGroupSpec{MinMember: int32(len(pods))}}
client.Create(ctx, pg)
for _, pod := range pods {
    pod.Labels["pod-group.scheduling.sigs.k8s.io"] = pg.Name
    client.Create(ctx, pod)
}
// Permit gate holds all pods until minMember are schedulable
// All-or-nothing: either all schedule or all wait',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-7',
    'Fluent Controller Builder: giant constructors with positional parameters are unclear. Fluent builder with Complete() for deferred validation. Self-documenting, fails fast with clear errors.',
    'ctrl := NewController(ctx, name, impl, fn1, dur1, fn2, dur2)',
    'ctrl, err := NewBuilder(ctx, "certificates").
    For(certificateController).
    With(renewalCheck, 10*time.Minute).
    With(expiryCheck, 1*time.Hour).
    Complete()',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-8',
    'Application Sync State Machine: scattered sync state flags are hard to reason about. Explicit state machine (OutOfSync, Syncing, Synced, SyncFailed) with health as separate dimension. Custom health via Lua for CRDs.',
    'type App struct {
    Synced    bool
    Healthy   bool
    SyncError string
}',
    'type SyncStatus string
const (
    SyncStatusOutOfSync SyncStatus = "OutOfSync"
    SyncStatusSynced    SyncStatus = "Synced"
    SyncStatusUnknown   SyncStatus = "Unknown"
)
type HealthStatus string
const (
    HealthStatusHealthy   HealthStatus = "Healthy"
    HealthStatusDegraded  HealthStatus = "Degraded"
    HealthStatusProgressing HealthStatus = "Progressing"
)
// Health and Sync are independent dimensions',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-9',
    'Multi-Cluster Secret-Based Registration: hard-coded cluster configs prevent dynamic addition and credential rotation. Cluster credentials in labeled K8s Secrets with server URL and auth. Watch secrets for dynamic add/remove.',
    'var clusters = map[string]*rest.Config{
    "prod": {Host: "https://prod:6443", BearerToken: "static-token"},
}',
    'func watchClusterSecrets(ctx context.Context) {
    informer := cache.NewFilteredListWatchFromClient(
        client, "secrets", "argocd",
        fields.OneTermEqualSelector("metadata.labels.argocd.argoproj.io/secret-type", "cluster"),
    )
    // Dynamic add/remove/update clusters from secrets
    // Bearer token or exec-based credential plugins for rotation
}',
    'trained',
    'kubernetes'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8S-10',
    'Fan-Out Watch Proxy for Scale: N-node cluster = N API server watches, overwhelming etcd at 100+ nodes. Aggregation proxy with single watch per resource, snapshot-and-delta for reconnection.',
    'func (a *Agent) Start() {
    a.informer = cache.NewInformer(a.client, ...)
    // 500 nodes = 500 watch connections
}',
    'type Typha struct {
    watchers map[resourceType]*watchMultiplexer
}
func (t *Typha) Subscribe(ctx context.Context, rt resourceType) <-chan Event {
    return t.watchers[rt].AddSubscriber(ctx)
    // 1 watch per resource type, fanned to all subscribers
}',
    'trained',
    'kubernetes'
);
