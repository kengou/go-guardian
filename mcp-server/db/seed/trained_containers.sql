-- Seed: trained container runtime patterns CRT-1 through CRT-6
-- Source: containerd, Podman, Docker Compose

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CRT-1',
    'Dependency-Aware Plugin Registration: hard-coded initialization order breaks on reordering. Declarative plugin registration with Requires field lets the framework resolve order via topological sort.',
    'func main() {
    storage := NewStorage()
    runtime := NewRuntime(storage)
    network := NewNetwork(runtime)
    api := NewAPI(runtime, network, storage)
}',
    'registry.Register(&plugin.Registration{
    Type:     plugins.GRPCPlugin,
    ID:       "containers",
    Requires: []plugin.Type{plugins.ServicePlugin},
    InitFn: func(ic *plugin.InitContext) (any, error) {
        svc, err := ic.GetByID(plugins.ServicePlugin, "containers-service")
        if err != nil { return nil, err }
        return &service{local: svc.(api.ContainersClient)}, nil
    },
})',
    'trained',
    'container-runtime'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CRT-2',
    'gRPC Service Delegation Layer: business logic mixed into gRPC handlers is untestable. Thin gRPC service wrapping a local api.Client implementation with UnimplementedServer embedding for forward compatibility.',
    'func (s *service) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
    // 200 lines of business logic mixed with transport
    container, err := s.db.Query(ctx, req.Id)
    // ...
}',
    'type service struct {
    local api.ContainersClient
    api.UnimplementedContainersServer
}
var _ api.ContainersServer = &service{}

func (s *service) Get(ctx context.Context, req *api.GetContainerRequest) (*api.GetContainerResponse, error) {
    return s.local.Get(ctx, req) // delegate to business logic
}',
    'trained',
    'container-runtime'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CRT-3',
    'Content-Addressable Atomic Writes: direct file writes leave partial content visible on crash. Atomic staged write: temp file, write with digest verification, os.Rename to final path.',
    'func (s *store) Write(ref string, data []byte) error {
    return os.WriteFile(filepath.Join(s.root, ref), data, 0644)
    // Partial writes visible, no integrity check
}',
    'func (s *store) writeToCompletion(dgst digest.Digest, data io.Reader) error {
    tmp, _ := os.CreateTemp(s.ingestDir, "tmp-")
    defer os.Remove(tmp.Name())
    verifier := dgst.Verifier()
    io.Copy(io.MultiWriter(tmp, verifier), data)
    tmp.Close()
    if !verifier.Verified() {
        return fmt.Errorf("integrity check failed")
    }
    return os.Rename(tmp.Name(), blobPath(dgst))
}',
    'trained',
    'container-runtime'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CRT-4',
    'Lease-Protected Garbage Collection: resources created without GC protection can be deleted between fetch and reference. Acquire lease before creating resources; content under lease context is GC-protected.',
    'func pullImage(ctx context.Context, ref string) error {
    manifest, _ := resolve(ctx, ref)
    // GC can delete layers here
    for _, layer := range manifest.Layers {
        download(ctx, layer) // fails: already GC''d
    }
}',
    'func pullImage(ctx context.Context, ref string) error {
    lease, _ := leases.Create(ctx, leases.WithRandomID(),
        leases.WithExpiration(24*time.Hour))
    defer leases.Delete(ctx, lease, leases.SynchronousDelete())
    ctx = leases.WithLease(ctx, lease.ID)
    manifest, _ := resolve(ctx, ref)
    for _, layer := range manifest.Layers {
        download(ctx, layer) // safe: lease protects from GC
    }
    return createImageRef(ctx, ref, manifest)
}',
    'trained',
    'container-runtime'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CRT-5',
    'Context-Propagated Namespace Isolation: namespace as function parameter pollutes every signature. Namespace in context lets the store layer extract it transparently for all queries.',
    'func GetContainer(ns, id string) (*Container, error) {
    return db.Query("SELECT * FROM containers WHERE ns = ? AND id = ?", ns, id)
}',
    'ctx = namespaces.WithNamespace(ctx, ns)
// Store layer extracts namespace from context
func GetContainer(ctx context.Context, id string) (*Container, error) {
    ns, err := namespaces.NamespaceRequired(ctx)
    if err != nil { return nil, err }
    return db.Query("... WHERE ns = ? AND id = ?", ns, id)
}',
    'trained',
    'container-runtime'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CRT-6',
    'Rootless Execution: requiring root for container operations is a security risk. Detect rootless mode, create user namespace mappings, fall back gracefully when rootless features are unavailable.',
    'func createContainer(spec *Spec) error {
    // Assumes root privileges
    return syscall.Mount(spec.Source, spec.Target, "overlay", 0, opts)
}',
    'func createContainer(spec *Spec) error {
    if rootless.IsRootless() {
        spec.Linux.UIDMappings = rootless.MappingsForUser(os.Getuid())
        spec.Linux.GIDMappings = rootless.MappingsForGroup(os.Getgid())
    }
    return mount(spec) // works both as root and rootless
}',
    'trained',
    'container-runtime'
);
