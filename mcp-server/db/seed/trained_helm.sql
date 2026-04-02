-- Seed: trained Helm chart patterns HELM-1 through HELM-15
-- Source: Prometheus (kube-prometheus-stack), Grafana, Istio, Linkerd2, Traefik, ArgoCD,
--         cert-manager, Cilium, Calico, Kyverno, OPA/Gatekeeper, Flux2, Crossplane, CoreDNS

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-1',
    'Standard Kubernetes Labels: custom labels like app: myapp break ecosystem tooling (kube-prometheus, ArgoCD, Lens). Use the app.kubernetes.io/* label set from the Kubernetes recommended labels specification.',
    'metadata:
  labels:
    app: my-service
    version: "1.0"
    team: platform',
    'metadata:
  labels:
    app.kubernetes.io/name: my-service
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/version: {{ .Chart.AppVersion }}
    app.kubernetes.io/component: server
    app.kubernetes.io/part-of: my-platform
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "chart.chart" . }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-2',
    'Resource Limits Always Set: pods without resource requests are scheduled with best-effort QoS. They are killed first under memory pressure. Always set requests (scheduling guarantee) and limits (ceiling) with sensible defaults in values.yaml.',
    'spec:
  containers:
    - name: server
      image: my-service:latest
      # no resources section — best-effort QoS, killed first under pressure',
    'spec:
  containers:
    - name: server
      image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
      resources:
        requests:
          cpu: {{ .Values.resources.requests.cpu | default "100m" }}
          memory: {{ .Values.resources.requests.memory | default "128Mi" }}
        limits:
          cpu: {{ .Values.resources.limits.cpu | default "500m" }}
          memory: {{ .Values.resources.limits.memory | default "256Mi" }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-3',
    'Security Context: pods without security context run as root with full capabilities. Violates Pod Security Standards (restricted). Set runAsNonRoot, readOnlyRootFilesystem, drop ALL capabilities, and disallow privilege escalation.',
    'spec:
  containers:
    - name: server
      image: my-service:latest
      # no securityContext — runs as root, all capabilities, writable filesystem',
    'spec:
  securityContext:
    runAsNonRoot: true
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: server
      image: my-service:latest
      securityContext:
        runAsNonRoot: true
        readOnlyRootFilesystem: true
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-4',
    'Image Tag Pinning: using :latest or omitting the tag means the image version is undefined and can change between pod restarts. Always use a specific tag defaulting to Chart.AppVersion. Never use :latest in production.',
    'spec:
  containers:
    - name: server
      image: my-org/my-service:latest
      # or worse: image: my-org/my-service (defaults to :latest)
      imagePullPolicy: Always',
    'spec:
  containers:
    - name: server
      image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
      imagePullPolicy: {{ .Values.image.pullPolicy | default "IfNotPresent" }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-5',
    'ConfigMap/Secret Rollout Trigger: updating a ConfigMap or Secret does not restart pods that mount it. Pods keep serving stale config until manually restarted. Add a checksum annotation to trigger rolling updates.',
    'spec:
  template:
    metadata:
      labels:
        app: my-service
    # ConfigMap changes are silently ignored — stale config until manual restart',
    'spec:
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-6',
    'Template Helpers in _helpers.tpl: duplicating label, name, and selector logic across every template is error-prone and inconsistent. Define helpers once in _helpers.tpl and use include everywhere.',
    '# deployment.yaml
metadata:
  name: {{ .Release.Name }}-my-service
  labels:
    app: my-service
    chart: {{ .Chart.Name }}-{{ .Chart.Version }}
# service.yaml (duplicated, slightly different)
metadata:
  name: {{ .Release.Name }}-my-service
  labels:
    app: my-service',
    '# _helpers.tpl
{{- define "chart.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "chart.labels" -}}
{{ include "chart.selectorLabels" . }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
# deployment.yaml
metadata:
  name: {{ include "chart.fullname" . }}
  labels: {{- include "chart.labels" . | nindent 4 }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-7',
    'PodDisruptionBudget for HA: multiple replicas without PDB can all be evicted simultaneously during node drain or cluster upgrade, causing complete downtime. Always create PDB for HA deployments.',
    '# deployment.yaml
spec:
  replicas: {{ .Values.replicaCount }}
  # no PDB — all replicas evicted at once during node drain',
    '# pdb.yaml
{{- if gt (int .Values.replicaCount) 1 }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "chart.fullname" . }}
spec:
  minAvailable: 1
  selector:
    matchLabels: {{- include "chart.selectorLabels" . | nindent 6 }}
{{- end }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-8',
    'Dedicated ServiceAccount: using the default service account exposes the token to all pods in the namespace. Create a dedicated SA with automountServiceAccountToken: false unless the pod actually needs API access.',
    'spec:
  # uses default service account — token auto-mounted, shared with all pods
  containers:
    - name: server',
    '# serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "chart.fullname" . }}
  annotations:
    {{- toYaml .Values.serviceAccount.annotations | nindent 4 }}
automountServiceAccountToken: {{ .Values.serviceAccount.automount | default false }}
---
# deployment.yaml
spec:
  serviceAccountName: {{ include "chart.fullname" . }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-9',
    'Health Probes: pods without probes receive traffic before ready and are never restarted when hung. Use readiness (traffic gating), liveness (restart on hang), and startup (slow init tolerance) probes.',
    'spec:
  containers:
    - name: server
      ports:
        - containerPort: 8080
      # no probes — receives traffic before ready, never restarted if hung',
    'spec:
  containers:
    - name: server
      ports:
        - containerPort: 8080
      startupProbe:
        httpGet:
          path: /healthz
          port: 8080
        failureThreshold: 30
        periodSeconds: 2
      readinessProbe:
        httpGet:
          path: /readyz
          port: 8080
        periodSeconds: 10
      livenessProbe:
        httpGet:
          path: /healthz
          port: 8080
        periodSeconds: 30
        failureThreshold: 3',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-10',
    'RBAC Least Privilege: ClusterRole with wildcard resources or verbs grants god-mode access. Use namespaced Roles where possible, list specific resources and verbs. cert-manager, Kyverno, and Flux all use fine-grained RBAC.',
    'apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]',
    'apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: {{ .Release.Namespace }}
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "update", "patch"]',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-11',
    'Values Schema Validation: invalid values cause cryptic template rendering errors at deploy time. Provide values.schema.json for type checking, required fields, and enum constraints. Catches errors at helm install, not at pod crash.',
    '# values.yaml with no validation
replicaCount: "not-a-number"  # template renders, pod fails
image:
  tag: ""  # empty tag deploys :latest silently',
    '# values.schema.json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "required": ["image"],
  "properties": {
    "replicaCount": { "type": "integer", "minimum": 1 },
    "image": {
      "required": ["repository"],
      "properties": {
        "repository": { "type": "string" },
        "tag": { "type": "string", "minLength": 1 }
      }
    }
  }
}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-12',
    'NetworkPolicy Default Deny: pods without NetworkPolicy accept traffic from any source in the cluster. Default deny ingress+egress, then explicitly allow required paths. Critical for multi-tenant and compliance environments.',
    '# no NetworkPolicy — pod accepts traffic from anywhere in cluster',
    '# networkpolicy.yaml
{{- if .Values.networkPolicy.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "chart.fullname" . }}
spec:
  podSelector:
    matchLabels: {{- include "chart.selectorLabels" . | nindent 6 }}
  policyTypes: ["Ingress", "Egress"]
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: api-gateway
      ports:
        - port: 8080
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 53
          protocol: UDP
{{- end }}',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-13',
    'Pod Anti-Affinity for HA: all replicas on the same node means a single node failure takes down the entire service. Use pod anti-affinity to spread replicas across nodes.',
    'spec:
  replicas: 3
  template:
    spec:
      # all 3 replicas can land on same node — single point of failure',
    'spec:
  replicas: {{ .Values.replicaCount }}
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels: {{- include "chart.selectorLabels" . | nindent 20 }}
                topologyKey: kubernetes.io/hostname',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-14',
    'Graceful Shutdown Configuration: default terminationGracePeriodSeconds (30s) may be too short for connection draining. Add preStop hook to stop accepting new connections and drain existing ones before SIGTERM.',
    'spec:
  terminationGracePeriodSeconds: 30  # default
  containers:
    - name: server
      # SIGTERM sent immediately, connections dropped mid-request',
    'spec:
  terminationGracePeriodSeconds: {{ .Values.terminationGracePeriodSeconds | default 60 }}
  containers:
    - name: server
      lifecycle:
        preStop:
          exec:
            command: ["/bin/sh", "-c", "sleep 5"]
      # sequence: preStop (5s drain) → SIGTERM → graceful shutdown',
    'trained',
    'helm'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'HELM-15',
    'NOTES.txt Post-Install Guidance: users installing the chart have no idea how to verify it works or access the service. NOTES.txt provides post-install instructions, port-forward commands, and verification steps.',
    '# no NOTES.txt — user runs helm install and has no idea what to do next',
    '# templates/NOTES.txt
{{- if .Values.ingress.enabled }}
Access {{ include "chart.fullname" . }} at:
  {{- range .Values.ingress.hosts }}
  http://{{ .host }}
  {{- end }}
{{- else }}
Get the application URL by running:
  kubectl port-forward svc/{{ include "chart.fullname" . }} 8080:{{ .Values.service.port }}
  Then open: http://localhost:8080
{{- end }}

Verify deployment:
  kubectl get pods -l "{{ include "chart.selectorLabels" . }}"',
    'trained',
    'helm'
);
