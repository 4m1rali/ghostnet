package bypass

import (
	"context"
	"fmt"
	"net"
	"plugin"
	"sync"
	"time"
)

type PluginParams struct {
	SrcIP     net.IP
	DstIP     net.IP
	SrcPort   uint16
	DstPort   uint16
	SynSeq    uint32
	SynAckSeq uint32
	SynTime   time.Time
	FakeSNI   string
	Options   map[string]string
}

type Plugin interface {
	Name() string
	Inject(ctx context.Context, p PluginParams) error
	Close() error
}

type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin)}
}

func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	r.plugins[p.Name()] = p
	r.mu.Unlock()
}

func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	p, ok := r.plugins[name]
	r.mu.RUnlock()
	return p, ok
}

func (r *Registry) LoadSharedPlugin(path string) error {
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("plugin: open %q: %w", path, err)
	}
	sym, err := p.Lookup("NewPlugin")
	if err != nil {
		return fmt.Errorf("plugin: lookup NewPlugin in %q: %w", path, err)
	}
	factory, ok := sym.(func() Plugin)
	if !ok {
		return fmt.Errorf("plugin: NewPlugin in %q has wrong type", path)
	}
	r.Register(factory())
	return nil
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.plugins))
	for n := range r.plugins {
		names = append(names, n)
	}
	return names
}
