-- Seed: trained testing patterns TEST-7 through TEST-10
-- Source: Kubernetes, Prometheus, Grafana, Perses, Greenhouse, VM Operator

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-7',
    'time.Sleep in Tests: using fixed sleeps to wait for asynchronous operations is slow, flaky, and hides timing bugs. Kubernetes uses wait.PollUntilContextCancel. Prometheus uses channel signaling. Grafana uses require.Eventually.',
    'func TestAsyncOperation(t *testing.T) {
    go startWorker()
    time.Sleep(3 * time.Second) // slow, flaky, wastes CI time
    result := getResult()
    require.Equal(t, "done", result)
}',
    'func TestAsyncOperation(t *testing.T) {
    go startWorker()

    // Option 1: require.Eventually (testify)
    require.Eventually(t, func() bool {
        return getResult() == "done"
    }, 10*time.Second, 100*time.Millisecond)

    // Option 2: wait.PollUntilContextCancel (K8s ecosystem)
    err := wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 10*time.Second, true,
        func(ctx context.Context) (bool, error) {
            return getResult() == "done", nil
        })
    require.NoError(t, err)
}',
    'trained',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-8',
    'Missing Goroutine Leak Detection: test suites for packages that spawn goroutines should use goleak to detect leaks. Prometheus uses testutil.TolerantVerifyLeak. Kubernetes uses goleak in controller packages.',
    'func TestMain(m *testing.M) {
    os.Exit(m.Run())
}',
    'import "go.uber.org/goleak"

func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}

// For individual tests (when TestMain is not available):
func TestWorker(t *testing.T) {
    defer goleak.VerifyNone(t)
    w := NewWorker()
    w.Start()
    w.Stop()
    // goleak verifies no goroutines leaked after Stop()
}',
    'trained',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-9',
    'Missing Controller Test Fixture: controller tests that start a full manager are slow, flaky, and hard to isolate. Kubernetes uses a fixture pattern with fake.NewSimpleClientset, pre-populated informer caches, and injectable syncHandler. VM Operator directly instantiates reconcilers.',
    'func TestController(t *testing.T) {
    // Starts full manager — slow, flaky, hard to isolate
    mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
    require.NoError(t, err)
    err = (&Reconciler{}).SetupWithManager(mgr)
    require.NoError(t, err)
    go mgr.Start(ctx)
    time.Sleep(2 * time.Second) // wait for manager
    // test logic
}',
    'type fixture struct {
    t       testing.TB
    client  *fake.Clientset
    objects []runtime.Object
}

func newFixture(t testing.TB) *fixture {
    return &fixture{t: t}
}

func (f *fixture) newController(ctx context.Context) (*Controller, error) {
    f.client = fake.NewSimpleClientset(f.objects...)
    informers := informers.NewSharedInformerFactory(f.client, 0)
    c, err := NewController(ctx, informers, f.client)
    if err != nil { return nil, err }
    c.syncHandler = c.syncDeployment // injectable for testing
    c.listerSynced = func() bool { return true } // bypass cache sync
    // pre-populate informer caches
    for _, obj := range f.objects {
        informers.Apps().V1().Deployments().Informer().GetIndexer().Add(obj)
    }
    return c, nil
}',
    'trained',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-10',
    'Assert Instead of Require: using assert.NoError followed by using the result causes nil pointer panics when the assertion fails because assert does not stop the test. Prometheus bans testify/assert entirely via depguard.',
    'func TestFetch(t *testing.T) {
    user, err := fetchUser(ctx, 1)
    assert.NoError(t, err) // test continues even if err != nil
    assert.Equal(t, "alice", user.Name) // panic: user is nil
}',
    'func TestFetch(t *testing.T) {
    user, err := fetchUser(ctx, 1)
    require.NoError(t, err) // test stops immediately if err != nil
    require.Equal(t, "alice", user.Name) // safe: user is non-nil
}',
    'trained',
    'testing'
);
