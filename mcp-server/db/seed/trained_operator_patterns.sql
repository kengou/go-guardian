-- Seed: trained operator patterns OP-1 through OP-6
-- Source: Kubernetes, Greenhouse, VM Operator

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-1',
    'Mutating Cached Objects: modifying an object returned from a lister or informer cache without DeepCopy corrupts the shared cache. All controllers reading the same object will see the mutation. Kubernetes comments: "Deep-copy otherwise we are mutating our cache."',
    'func (c *Controller) sync(ctx context.Context, key string) error {
    deployment, err := c.dLister.Deployments(ns).Get(name)
    if err != nil { return err }

    deployment.Spec.Replicas = ptr.To(int32(3)) // WRONG: mutates shared cache
    _, err = c.client.AppsV1().Deployments(ns).Update(ctx, deployment, metav1.UpdateOptions{})
    return err
}',
    'func (c *Controller) sync(ctx context.Context, key string) error {
    deployment, err := c.dLister.Deployments(ns).Get(name)
    if err != nil { return err }

    d := deployment.DeepCopy() // safe: working on an independent copy
    d.Spec.Replicas = ptr.To(int32(3))
    _, err = c.client.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
    return err
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-2',
    'Missing Tombstone Handling in Delete Event Handlers: when an informer misses a delete event, it sends a DeletedFinalStateUnknown tombstone instead of the actual object. A direct type assertion panics. Every Kubernetes controller handles this.',
    'func (c *Controller) onDelete(obj interface{}) {
    d := obj.(*appsv1.Deployment) // panics on tombstone
    c.enqueue(d)
}',
    'func (c *Controller) onDelete(obj interface{}) {
    d, ok := obj.(*appsv1.Deployment)
    if !ok {
        tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
        if !ok {
            utilruntime.HandleError(fmt.Errorf("unexpected object type %T", obj))
            return
        }
        d, ok = tombstone.Obj.(*appsv1.Deployment)
        if !ok {
            utilruntime.HandleError(fmt.Errorf("tombstone object is not a Deployment: %T", tombstone.Obj))
            return
        }
    }
    c.enqueue(d)
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-3',
    'Owner Reference Without UID Check: resolving an owner reference by name alone can match a different object that was recreated with the same name after the original was deleted. Kubernetes controllers always validate both name AND UID.',
    'func (c *Controller) resolveOwner(ns string, ref *metav1.OwnerReference) *appsv1.Deployment {
    d, err := c.dLister.Deployments(ns).Get(ref.Name)
    if err != nil { return nil }
    return d // WRONG: could be a different deployment with the same name
}',
    'func (c *Controller) resolveOwner(ns string, ref *metav1.OwnerReference) *appsv1.Deployment {
    if ref.Kind != "Deployment" { return nil }
    d, err := c.dLister.Deployments(ns).Get(ref.Name)
    if err != nil { return nil }
    if d.UID != ref.UID { return nil } // critical: verify UID matches
    return d
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-4',
    'Status Update Instead of Patch: using Status().Update() does a full replace of the status subresource, which races with other controllers writing conditions to the same object and can lose data. Greenhouse uses MergeFrom patches at the end of every reconcile.',
    'func (r *Reconciler) updateStatus(ctx context.Context, obj *myv1.Resource) error {
    obj.Status.Phase = "Ready"
    return r.Status().Update(ctx, obj) // full replace — races with other controllers
}',
    'func (r *Reconciler) updateStatus(ctx context.Context, obj *myv1.Resource) error {
    old := obj.DeepCopy()
    obj.Status.Phase = "Ready"
    return r.Status().Patch(ctx, obj, client.MergeFrom(old)) // atomic patch — no data loss
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-5',
    'Finalizer After External Resource Creation: creating an external resource before adding a finalizer means a crash between the two steps leaks the resource with no cleanup path. Both Greenhouse and VM Operator add finalizers before creating child resources.',
    'func (r *Reconciler) EnsureCreated(ctx context.Context, obj *myv1.Resource) error {
    // Create external resource first
    if err := r.createCloudResource(ctx, obj); err != nil {
        return err
    }
    // Add finalizer after — crash here leaks the cloud resource
    controllerutil.AddFinalizer(obj, finalizerName)
    return r.Update(ctx, obj)
}',
    'func (r *Reconciler) EnsureCreated(ctx context.Context, obj *myv1.Resource) error {
    // Add finalizer first — guarantees cleanup path exists
    if !controllerutil.ContainsFinalizer(obj, finalizerName) {
        controllerutil.AddFinalizer(obj, finalizerName)
        if err := r.Update(ctx, obj); err != nil {
            return fmt.Errorf("add finalizer: %w", err)
        }
    }
    // Now safe to create external resource
    return r.createCloudResource(ctx, obj)
}',
    'trained',
    'operator'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OP-6',
    'Unbounded Requeue on Transient Errors: returning Requeue: true for every error creates a hot loop under persistent failures. Kubernetes uses TypedRateLimitingQueue with maxRetries=15 and exponential backoff. VM Operator classifies errors and applies custom rate limiters.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.doWork(ctx, req); err != nil {
        return ctrl.Result{Requeue: true}, nil // hot loop on persistent failure
    }
    return ctrl.Result{}, nil
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.doWork(ctx, req); err != nil {
        // Return the error — controller-runtime applies exponential backoff
        return ctrl.Result{}, fmt.Errorf("reconcile %s: %w", req.NamespacedName, err)
    }
    return ctrl.Result{}, nil
}

// For periodic re-checks (not errors), use RequeueAfter:
// return ctrl.Result{RequeueAfter: 30 * time.Second}, nil',
    'trained',
    'operator'
);
