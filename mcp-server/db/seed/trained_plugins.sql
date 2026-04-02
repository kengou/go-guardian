-- Seed: trained plugin architecture patterns PLUG-1 through PLUG-5
-- Source: CoreDNS, Pulumi, containerd

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'PLUG-1',
    'Functional Middleware Chain: plugin manager with if/else dispatch is rigid and untestable. Each plugin is a func(Handler) Handler. Chain built by wrapping backwards. Compiled once at startup for zero per-request overhead.',
    'type PluginManager struct {
    plugins map[string]Plugin
}
func (pm *PluginManager) Handle(r *dns.Msg) {
    if p, ok := pm.plugins["cache"]; ok { p.Process(r) }
    if p, ok := pm.plugins["forward"]; ok { p.Process(r) }
}',
    'type Plugin func(Handler) Handler
type Handler interface {
    ServeDNS(context.Context, dns.ResponseWriter, *dns.Msg) (int, error)
}
var stack plugin.Handler
for i := len(plugins) - 1; i >= 0; i-- {
    stack = plugins[i](stack)
}',
    'trained',
    'plugin-architecture'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'PLUG-2',
    'Directive-Ordered Plugin Registration: map-based registration has non-deterministic execution order. Single source-of-truth ordering list; plugins register via init() but execute in directive order.',
    'var plugins = map[string]SetupFunc{}
func Register(name string, setup SetupFunc) {
    plugins[name] = setup
}',
    'var Directives = []string{"log", "errors", "cache", "kubernetes", "forward"}
func init() { plugin.Register("forward", setup) }
func (c *Config) Handlers() []plugin.Handler {
    var hs []plugin.Handler
    for _, k := range Directives {
        if h := c.Handler(k); h != nil { hs = append(hs, h) }
    }
    return hs
}',
    'trained',
    'plugin-architecture'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'PLUG-3',
    'Per-Plugin Block-Scoped Configuration: monolithic config struct with 50+ fields for all plugins couples everything. Each plugin owns its setup function, parses its own config block, registers its own lifecycle hooks.',
    'type Config struct {
    ForwardTo []string
    CacheTTL  int
    MaxFails  int
    // 50 more fields
}',
    'func setup(c *caddy.Controller) error {
    fs, err := parseForward(c)
    if err != nil { return plugin.Error("forward", err) }
    c.OnStartup(func() error { return fs.OnStartup() })
    c.OnShutdown(func() error { return fs.OnShutdown() })
    dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
        fs.Next = next
        return fs
    })
    return nil
}',
    'trained',
    'plugin-architecture'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'PLUG-4',
    'UnimplementedProvider Embedding: bare interface implementations break when new methods are added upstream. Embed UnimplementedProvider so new methods auto-return Unimplemented. mustEmbed unexported method forces the embed.',
    'type MyProvider struct{} // breaks when upstream adds Read()
func (p *MyProvider) Create(ctx context.Context, req CreateRequest) (CreateResponse, error) {
    // ...
}',
    'type MyProvider struct {
    UnimplementedProvider // safe: new methods auto-return Unimplemented
}
func (p *MyProvider) Create(ctx context.Context, req CreateRequest) (CreateResponse, error) {
    // override only what you need
}',
    'trained',
    'plugin-architecture'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'PLUG-5',
    'Resource Model with Input/Output Types: plain structs for infrastructure resources have no dependency tracking, diffing, or preview. Separate Input (desired, may contain unknowns) from Output (resolved). Track dependencies via output references.',
    'type Server struct {
    IP   string
    Name string
}
func Create(s Server) error {
    return cloud.CreateVM(s.Name, s.IP)
}',
    'type ServerArgs struct {
    Name pulumi.StringInput
    IP   pulumi.StringInput // may be unknown during preview
}
type Server struct {
    pulumi.ResourceState
    IP pulumi.StringOutput // resolved after creation
}
// Dependencies tracked via output references
// Check/Diff/Create/Update/Delete lifecycle',
    'trained',
    'plugin-architecture'
);
