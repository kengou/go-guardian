-- Seed: trained Kubernetes resource manifest patterns K8SRES-1 through K8SRES-15
-- Source: Kubernetes (upstream), Kyverno, OPA/Gatekeeper, cert-manager, Crossplane, Flux2,
--         Gardener, Istio, Cilium, Calico, ArgoCD, Linkerd2, Traefik, containerd, Podman

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-1',
    'Pod Security Standards Enforcement: namespaces without Pod Security Admission labels allow privileged pods. Apply enforce: restricted for production namespaces. Kyverno and OPA/Gatekeeper can enforce the same via policies.',
    'apiVersion: v1
kind: Namespace
metadata:
  name: production
  # no pod security labels — privileged pods allowed',
    'apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/enforce-version: latest
    pod-security.kubernetes.io/warn: restricted
    pod-security.kubernetes.io/audit: restricted',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-2',
    'Standard Label Set: inconsistent labels break kube-prometheus service discovery, ArgoCD app tracking, and kubectl filtering. Use the Kubernetes recommended label set (app.kubernetes.io/*) on all resources.',
    'metadata:
  labels:
    app: api
    env: prod
    # non-standard — invisible to kube-prometheus, ArgoCD, Lens',
    'metadata:
  labels:
    app.kubernetes.io/name: api-server
    app.kubernetes.io/instance: api-prod
    app.kubernetes.io/version: "1.5.0"
    app.kubernetes.io/component: api
    app.kubernetes.io/part-of: my-platform
    app.kubernetes.io/managed-by: helm',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-3',
    'Resource Requests and Limits: pods without resource specs get best-effort QoS and are evicted first. Set requests for scheduling guarantees and limits as ceiling. For guaranteed QoS, set requests equal to limits.',
    'spec:
  containers:
    - name: app
      image: app:1.0
      # no resources — best-effort QoS, evicted first under memory pressure',
    'spec:
  containers:
    - name: app
      image: app:1.0
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
        limits:
          cpu: 500m
          memory: 256Mi
          ephemeral-storage: 100Mi',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-4',
    'Complete Container Security Context: containers without security context run as root (UID 0) with all Linux capabilities. Set the full restricted security context: non-root, read-only filesystem, no privilege escalation, drop ALL caps, seccomp.',
    'spec:
  containers:
    - name: app
      image: app:1.0
      # default: root, all capabilities, writable filesystem, no seccomp',
    'spec:
  securityContext:
    runAsNonRoot: true
    fsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: app
      image: app:1.0
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        readOnlyRootFilesystem: true
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-5',
    'RBAC Least Privilege: ClusterRoles with wildcard verbs or resources grant full cluster access. Use specific verbs on specific resources. Prefer namespaced Roles over ClusterRoles. cert-manager, Kyverno, and Flux all use fine-grained RBAC.',
    'apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-controller
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]',
    'apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-controller
rules:
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
  - apiGroups: ["my.domain"]
    resources: ["myresources"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["my.domain"]
    resources: ["myresources/status"]
    verbs: ["update", "patch"]',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-6',
    'NetworkPolicy Default Deny: pods without NetworkPolicy accept traffic from any pod in the cluster. Apply default deny for both ingress and egress, then add explicit allow rules. Critical for multi-tenant clusters.',
    '# no NetworkPolicy — any pod can reach any pod on any port',
    '# default deny all
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: production
spec:
  podSelector: {}
  policyTypes: ["Ingress", "Egress"]
---
# explicit allow
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-api
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: api
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: gateway
      ports:
        - port: 8080
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 53
          protocol: UDP',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-7',
    'Topology Spread Constraints: pod anti-affinity alone does not guarantee even distribution across zones. Use topologySpreadConstraints for balanced spreading across both zones and nodes. Istio and ArgoCD use this for HA.',
    'spec:
  affinity:
    podAntiAffinity:
      # only prevents co-location, does not ensure even distribution
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            topologyKey: kubernetes.io/hostname',
    'spec:
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          app.kubernetes.io/name: my-service
    - maxSkew: 1
      topologyKey: kubernetes.io/hostname
      whenUnsatisfiable: ScheduleAnyway
      labelSelector:
        matchLabels:
          app.kubernetes.io/name: my-service',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-8',
    'Graceful Shutdown with preStop: SIGTERM is sent immediately when a pod terminates, but kube-proxy may still route traffic to it for a few seconds. A preStop sleep gives kube-proxy time to remove the pod from endpoints.',
    'spec:
  terminationGracePeriodSeconds: 30
  containers:
    - name: app
      # SIGTERM sent immediately — receives traffic during deregistration window
      # clients see connection refused or 502 for ~5 seconds',
    'spec:
  terminationGracePeriodSeconds: 60
  containers:
    - name: app
      lifecycle:
        preStop:
          exec:
            command: ["sh", "-c", "sleep 5 && kill -SIGTERM 1"]
      # sequence:
      # 1. preStop runs (5s) — kube-proxy removes pod from endpoints
      # 2. SIGTERM sent — app starts graceful shutdown
      # 3. connections drain within remaining grace period',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-9',
    'CRD Structural Schema with CEL Validation: CRDs without OpenAPI schema accept any input — invalid configs crash controllers at runtime. Add structural schemas with CEL validation rules for complex constraints.',
    'apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
spec:
  versions:
    - name: v1
      schema:
        openAPIV3Schema:
          type: object
          # no properties defined — accepts anything',
    'apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
spec:
  versions:
    - name: v1
      schema:
        openAPIV3Schema:
          type: object
          required: ["spec"]
          properties:
            spec:
              type: object
              required: ["replicas"]
              properties:
                replicas:
                  type: integer
                  minimum: 1
                  maximum: 100
                schedule:
                  type: string
                  pattern: "^(@(annually|yearly|monthly|weekly|daily|hourly))|((\\S+\\s+){4}\\S+)$"
      additionalPrinterColumns:
        - name: Replicas
          type: integer
          jsonPath: .spec.replicas
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-10',
    'Admission Webhook Fail-Closed: webhooks with failurePolicy: Ignore silently skip validation when the webhook is unavailable. Use failurePolicy: Fail with appropriate timeoutSeconds and namespaceSelector to exclude kube-system.',
    'apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
webhooks:
  - name: validate.my.domain
    failurePolicy: Ignore  # webhook down = all requests pass unchecked
    timeoutSeconds: 30     # blocks API server for 30s on timeout
    # no namespaceSelector — validates kube-system too',
    'apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
webhooks:
  - name: validate.my.domain
    failurePolicy: Fail
    timeoutSeconds: 10
    namespaceSelector:
      matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: NotIn
          values: ["kube-system", "kube-public"]
    rules:
      - apiGroups: ["my.domain"]
        apiVersions: ["v1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["myresources"]
        scope: Namespaced',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-11',
    'Distinct Probe Endpoints: using the same path for liveness and readiness means a temporarily overloaded pod is killed instead of just removed from traffic. Use /healthz for liveness (is the process alive?) and /readyz for readiness (can it accept traffic?).',
    'spec:
  containers:
    - name: app
      livenessProbe:
        httpGet:
          path: /health
          port: 8080
      readinessProbe:
        httpGet:
          path: /health  # same as liveness — overloaded pod gets killed
          port: 8080',
    'spec:
  containers:
    - name: app
      startupProbe:
        httpGet:
          path: /healthz
          port: 8080
        failureThreshold: 30
        periodSeconds: 2
      livenessProbe:
        httpGet:
          path: /healthz     # is the process alive and not deadlocked?
          port: 8080
        periodSeconds: 30
        failureThreshold: 3
      readinessProbe:
        httpGet:
          path: /readyz      # can it handle traffic right now?
          port: 8080
        periodSeconds: 10
        failureThreshold: 1',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-12',
    'Priority Classes for Critical Components: system-critical pods (CNI, DNS, cert-manager) without PriorityClass can be preempted by user workloads. Assign system-cluster-critical or custom PriorityClass to infrastructure.',
    'apiVersion: apps/v1
kind: Deployment
metadata:
  name: cert-manager
spec:
  template:
    spec:
      # no priorityClassName — can be preempted by any user pod
      containers:
        - name: cert-manager',
    'apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: infra-critical
value: 1000000
globalDefault: false
description: "Infrastructure components that must not be preempted"
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      priorityClassName: infra-critical',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-13',
    'Ephemeral Storage Limits: containers without ephemeral-storage limits can fill the node disk with logs, temp files, or emptyDir volumes, causing node eviction of all pods. Set limits for containers that use temp storage.',
    'spec:
  containers:
    - name: app
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
        limits:
          cpu: 500m
          memory: 256Mi
        # no ephemeral-storage — pod can fill node disk',
    'spec:
  containers:
    - name: app
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
          ephemeral-storage: 50Mi
        limits:
          cpu: 500m
          memory: 256Mi
          ephemeral-storage: 200Mi',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-14',
    'Immutable ConfigMaps and Secrets: mutable ConfigMaps allow silent config drift. Immutable ConfigMaps reduce API server load (no watch updates) and prevent accidental changes. Create new versions with checksums for rollout.',
    'apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  config.yaml: |
    key: value
  # mutable — can be changed without pod restart, no audit trail',
    'apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config-v2a3f4  # versioned name from content hash
immutable: true
data:
  config.yaml: |
    key: value
# Deployment references specific version, change = new ConfigMap + rolling update',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-15',
    'Finalizer Discipline: finalizers without cleanup logic or timeout prevent resource deletion forever (stuck in Terminating state). Always check DeletionTimestamp in reconcile, clean up external resources, then remove the finalizer.',
    'apiVersion: my.domain/v1
kind: MyResource
metadata:
  finalizers:
    - my.domain/cleanup
  # finalizer added but controller has no cleanup logic
  # or: cleanup logic fails permanently, resource stuck forever',
    '// In reconcile:
if !obj.DeletionTimestamp.IsZero() {
    // 1. Clean up external resources (with timeout)
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    if err := r.cleanupExternal(ctx, obj); err != nil {
        return ctrl.Result{RequeueAfter: time.Minute}, err
    }
    // 2. Remove finalizer after successful cleanup
    controllerutil.RemoveFinalizer(obj, finalizerName)
    return ctrl.Result{}, r.Update(ctx, obj)
}
// Before creating external resources, add finalizer
if !controllerutil.ContainsFinalizer(obj, finalizerName) {
    controllerutil.AddFinalizer(obj, finalizerName)
    return ctrl.Result{}, r.Update(ctx, obj)
}',
    'trained',
    'k8s-resources'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'K8SRES-16',
    'fsGroupChangePolicy OnRootMismatch: the default fsGroupChangePolicy (Always) recursively chown/chgrps the entire persistent volume on every pod mount. For StatefulSets with large PVs this causes multi-minute startup delays. Set OnRootMismatch to only fix permissions when the root directory group does not match. Discovered at Cloudflare where it saved 600+ engineering hours/year on Atlantis restarts.',
    'spec:
  securityContext:
    fsGroup: 65532
    # fsGroupChangePolicy defaults to Always
    # — recursive chown on every mount, 30-minute startup on large PVs',
    'spec:
  securityContext:
    fsGroup: 65532
    fsGroupChangePolicy: OnRootMismatch
    # only changes permissions when root dir group mismatches
    # — startup drops from 30 minutes to ~30 seconds',
    'trained',
    'k8s-resources'
);
