-- Seed: trained GitOps patterns GITOPS-1 through GITOPS-6
-- Source: Flux2, Crossplane, Gardener

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'GITOPS-1',
    'Direct Source Coupling: consumers that directly reference source objects (GitRepository, HelmRepository) cannot switch source types without code changes. Flux uses an artifact-based indirection where sources produce versioned Artifact objects that consumers reference.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var gitRepo sourcev1.GitRepository
    if err := r.Get(ctx, req.NamespacedName, &gitRepo); err != nil {
        return ctrl.Result{}, err
    }
    data, err := r.fetchFromGit(gitRepo.Spec.URL, gitRepo.Spec.Ref)
    // Tightly coupled to GitRepository — cannot use HelmRepository or Bucket
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get artifact from any source type (Git, Helm, Bucket, OCI)
    artifact, err := r.getSourceArtifact(ctx, obj.Spec.SourceRef)
    if err != nil {
        return ctrl.Result{}, err
    }
    data, err := r.fetchArtifact(artifact.URL, artifact.Digest)
    // Decoupled: works with any source that produces artifacts
}',
    'trained',
    'gitops'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'GITOPS-2',
    'Reconciling Without Checking Dependencies: applying a Kustomization before its dependencies are ready causes failures that trigger unnecessary retries. Flux checks all dependency resources are in Ready condition before proceeding.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Apply immediately — if dependency Kustomization is not ready, this fails
    return r.apply(ctx, obj)
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.checkDependencies(ctx, obj.Spec.DependsOn); err != nil {
        // Dependencies not ready — requeue at dependency interval, not error backoff
        conditions.MarkFalse(obj, meta.ReadyCondition, "DependencyNotReady", err.Error())
        return ctrl.Result{RequeueAfter: r.dependencyRequeueInterval}, nil
    }
    return r.apply(ctx, obj)
}',
    'trained',
    'gitops'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'GITOPS-3',
    'Unrestricted Cross-Namespace References: allowing resources in one namespace to reference sources or secrets in another namespace bypasses multi-tenant isolation. Flux enforces NoCrossNamespaceRefs with explicit ACL checks.',
    'func (r *Reconciler) getSource(ctx context.Context, ref sourcev1.CrossNamespaceSourceReference) (*sourcev1.Artifact, error) {
    // Any namespace can reference any source — no tenant isolation
    return r.getArtifact(ctx, types.NamespacedName{
        Name: ref.Name, Namespace: ref.Namespace,
    })
}',
    'func (r *Reconciler) getSource(ctx context.Context, obj *kustomizev1.Kustomization, ref sourcev1.CrossNamespaceSourceReference) (*sourcev1.Artifact, error) {
    if r.noCrossNamespaceRefs && ref.Namespace != "" && ref.Namespace != obj.Namespace {
        return nil, acl.AccessDeniedError(fmt.Errorf(
            "cross-namespace reference to %s/%s denied by NoCrossNamespaceRefs policy",
            ref.Namespace, ref.Name,
        ))
    }
    ns := ref.Namespace
    if ns == "" {
        ns = obj.Namespace // default to same namespace
    }
    return r.getArtifact(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns})
}',
    'trained',
    'gitops'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'GITOPS-4',
    'Alerting Only on Failure: notifying only when resources fail misses recovery events. Flux detects FailureRecovery by comparing old and new conditions, enabling alerts when a previously-failed resource becomes healthy.',
    'func (r *Reconciler) notify(ctx context.Context, obj *myv1.Resource, err error) {
    if err != nil {
        r.eventRecorder.Event(obj, "Warning", "ReconcileError", err.Error())
    }
    // No notification on recovery — operators do not know when issues resolve
}',
    'func (r *Reconciler) notify(ctx context.Context, oldObj, newObj *myv1.Resource, err error) {
    if err != nil {
        r.eventRecorder.Event(newObj, "Warning", "ReconcileError", err.Error())
    }
    // Detect recovery: fail conditions cleared between old and new
    if FailureRecovery(oldObj, newObj, failConditions) {
        r.eventRecorder.Event(newObj, "Normal", "ReconcileRecovered",
            "Resource recovered from previous failure")
    }
}',
    'trained',
    'gitops'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'GITOPS-5',
    'No Drift Detection Requeue: reconciling only on events means configuration drift between events goes undetected. Flux always requeues (even on success) at the poll interval to detect and correct drift.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.apply(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, nil // no requeue — drift between events goes undetected
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.apply(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    // Always requeue at poll interval to detect drift
    return ctrl.Result{RequeueAfter: jitter(r.pollInterval)}, nil
}',
    'trained',
    'gitops'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'GITOPS-6',
    'Separate Status Update Timeout: using the same context timeout for reconciliation work and the final status update means the status update can fail after the reconcile completes if the context expires. Crossplane uses a longer timeout for status updates.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
    defer cancel()

    if err := r.doWork(ctx); err != nil {
        return ctrl.Result{}, err
    }
    // Status update uses same ctx — may fail if timeout nearly expired
    return ctrl.Result{}, r.Status().Update(ctx, obj)
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    workCtx, workCancel := context.WithTimeout(ctx, 2*time.Minute)
    defer workCancel()

    if err := r.doWork(workCtx); err != nil {
        return ctrl.Result{}, err
    }

    // Status update gets extra time beyond the work timeout
    updateCtx, updateCancel := context.WithTimeout(ctx, 2*time.Minute+20*time.Second)
    defer updateCancel()
    return ctrl.Result{}, r.Status().Update(updateCtx, obj)
}',
    'trained',
    'gitops'
);
