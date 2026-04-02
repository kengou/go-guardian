-- Seed: trained advanced operator patterns OP-7 through OP-14
-- Source: Gardener, Flux2, Crossplane, Chaos-Mesh, Helm

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-7',
    'Linear Reconciliation for Complex Resources: reconciling a resource with many independent sub-tasks sequentially wastes time. Gardener builds a DAG of ~80 tasks with explicit dependencies and executes with maximum parallelism.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.deployInfra(ctx); err != nil { return ctrl.Result{}, err }
    if err := r.deployDNS(ctx); err != nil { return ctrl.Result{}, err }
    if err := r.deployControlPlane(ctx); err != nil { return ctrl.Result{}, err }
    if err := r.deployWorkers(ctx); err != nil { return ctrl.Result{}, err }
    // Sequential — deployDNS waits for deployInfra even though they are independent
    return ctrl.Result{}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    g := flow.NewGraph("shoot-reconcile")
    deployInfra := g.Add(flow.Task{Name: "deploy-infra", Fn: r.deployInfra})
    deployDNS := g.Add(flow.Task{Name: "deploy-dns", Fn: r.deployDNS})
    // Independent tasks run in parallel; dependent tasks wait
    deployCP := g.Add(flow.Task{
        Name:         "deploy-control-plane",
        Fn:           r.deployControlPlane,
        Dependencies: flow.NewTaskIDs(deployInfra),
    })
    deployWorkers := g.Add(flow.Task{
        Name:         "deploy-workers",
        Fn:           r.deployWorkers,
        Dependencies: flow.NewTaskIDs(deployCP),
    })
    return ctrl.Result{}, g.Execute(ctx)
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-8',
    'Ad-Hoc Condition and Requeue Logic in Reconcilers: scattering condition-setting and requeue decisions throughout the reconciler makes behavior unpredictable. Flux defines typed errors (Stalling, Waiting, Generic) that carry condition/requeue semantics, with a central ComputeReconcileResult function.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.doWork(ctx); err != nil {
        if isPermanent(err) {
            meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
                Type: "Stalled", Status: "True", Reason: "PermanentError",
            })
            return ctrl.Result{}, nil // ad-hoc: easy to forget conditions
        }
        return ctrl.Result{RequeueAfter: time.Minute}, err
    }
    return ctrl.Result{}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    result, err := r.doWork(ctx)
    // Central function examines error type → sets conditions + determines requeue
    return reconcile.ComputeReconcileResult(obj, result, err,
        r.requeueInterval, r.retryInterval)
}

// Error types carry semantics:
// &Stalling{Err: err}     → set Stalled condition, no requeue
// &Waiting{Err: err}      → set Reconciling condition, requeue with custom interval
// &Generic{Err: err}      → standard error, requeue with backoff',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-9',
    'Controllers Overwriting Each Others Conditions: multiple controllers writing conditions to the same object without ownership tracking causes lost updates. Flux declares owned conditions per controller, and a summarization helper computes Ready from owned conditions only.',
    'func (r *Reconciler) updateStatus(ctx context.Context, obj *myv1.Resource) error {
    meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
        Type: "Ready", Status: "True",
    })
    // Other controller also sets Ready — last writer wins, data lost
    return r.Status().Update(ctx, obj)
}',
    'var ownedConditions = summarize.Conditions{
    Target: meta.ReadyCondition,
    Owned: []string{
        FetchFailedCondition,
        BuildFailedCondition,
        StorageOperationFailedCondition,
    },
    NegativePolarity: []string{
        FetchFailedCondition,
        BuildFailedCondition,
    },
}

func (r *Reconciler) updateStatus(ctx context.Context, obj *myv1.Resource) error {
    // Only sets owned conditions; Ready computed from owned set
    return summarize.SummarizeAndPatch(ctx, r.patcher, obj, ownedConditions)
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-10',
    'Monolithic Action Handler: a single reconcile function that handles all possible actions via switch/case becomes unwieldy. Chaos-Mesh uses an action multiplexer that dispatches to per-action implementations via a registry.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    switch obj.Spec.Action {
    case "pod-kill":
        return r.handlePodKill(ctx, obj)
    case "network-delay":
        return r.handleNetworkDelay(ctx, obj)
    case "stress-cpu":
        return r.handleStressCPU(ctx, obj)
    // grows unbounded with each new action
    }
}',
    'type ChaosImpl interface {
    Apply(ctx context.Context, index int, records []*Record, obj InnerObject) (Phase, error)
    Recover(ctx context.Context, index int, records []*Record, obj InnerObject) (Phase, error)
}

// Register implementations
var registry = map[string]ChaosImpl{
    "pod-kill":      &PodKillImpl{},
    "network-delay": &NetworkDelayImpl{},
    "stress-cpu":    &StressCPUImpl{},
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    impl, ok := registry[obj.Spec.Action]
    if !ok {
        return ctrl.Result{}, &ErrorUnknownAction{Action: obj.Spec.Action}
    }
    phase, err := impl.Apply(ctx, 0, obj.Status.Records, obj)
    return ctrl.Result{}, err
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-11',
    'No Circuit Breaker on Reconciler: a reconciler that keeps hitting a failing external dependency at full rate causes thundering herd and wastes resources. Crossplane wraps reconcilers with circuit breakers that open after repeated failures.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // External API down — every object requeues at full rate
    if err := r.callExternalAPI(ctx); err != nil {
        return ctrl.Result{}, err // exponential backoff per object, but N objects = N calls
    }
    return ctrl.Result{}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if state := r.breaker.GetState(); state.IsOpen {
        // Circuit open — skip external calls until breaker resets
        return ctrl.Result{RequeueAfter: r.backoffInterval}, nil
    }
    if err := r.callExternalAPI(ctx); err != nil {
        r.breaker.RecordFailure()
        return ctrl.Result{}, err
    }
    r.breaker.RecordSuccess()
    return ctrl.Result{}, nil
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-12',
    'Returning Conflict as Error: returning a conflict error from a reconciler triggers exponential backoff, but conflicts are expected and should be retried immediately. Crossplane and Flux check IsConflict and return Requeue: true instead.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.Update(ctx, obj); err != nil {
        return ctrl.Result{}, err // conflict triggers exponential backoff
    }
    return ctrl.Result{}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.Update(ctx, obj); err != nil {
        if apierrors.IsConflict(err) {
            return ctrl.Result{Requeue: true}, nil // immediate retry, no backoff
        }
        return ctrl.Result{}, fmt.Errorf("update: %w", err)
    }
    return ctrl.Result{}, nil
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-13',
    'Fixed Requeue Interval Without Jitter: all instances of a controller requeuing at the same fixed interval causes thundering herd. Crossplane and Flux add +/-10% jitter to poll intervals.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // All 1000 resources requeue at exactly 30s — thundering herd
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    return ctrl.Result{RequeueAfter: jitter(r.pollInterval)}, nil
}

// jitter adds +/- 10% randomness to prevent thundering herd
func jitter(d time.Duration) time.Duration {
    factor := 0.9 + rand.Float64()*0.2 // [0.9, 1.1)
    return time.Duration(float64(d) * factor)
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-14',
    'No Incremental Error Recovery: reconciling a complex resource where some subtasks failed previously but the reconciler has no memory of which tasks errored means it cannot perform targeted recovery. Gardener tracks error IDs across reconciliation cycles and clears them on success.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // No memory of previous errors — retries everything from scratch
    if err := r.step1(ctx); err != nil { return ctrl.Result{}, err }
    if err := r.step2(ctx); err != nil { return ctrl.Result{}, err }
    return ctrl.Result{}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    errCtx := errors.NewErrorContext("shoot", obj.Status.LastErrors)
    result := errors.HandleErrors(errCtx,
        func(taskID string) { clearErrorFromStatus(obj, taskID) }, // on success
        func(taskID string, err error) { recordError(obj, taskID, err) }, // on failure
        errors.Task{ID: "step1", Fn: r.step1},
        errors.Task{ID: "step2", Fn: r.step2},
    )
    return ctrl.Result{}, result.Error()
}',
    'trained',
    'operator'
);
