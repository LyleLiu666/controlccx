package connectors

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var (
	ErrInvalidConnector = errors.New("connectors: invalid connector")
	ErrConnectorExists  = errors.New("connectors: connector already exists")
	ErrConnectorMissing = errors.New("connectors: connector not found")
)

type Registry struct {
	mu   sync.RWMutex
	byID map[string]Connector
}

func NewRegistry() *Registry {
	return &Registry{byID: map[string]Connector{}}
}

func (r *Registry) Register(c Connector) error {
	if c == nil {
		return ErrInvalidConnector
	}
	name := normalizeName(c.Name())
	if name == "" {
		return ErrInvalidConnector
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[name]; exists {
		return ErrConnectorExists
	}
	r.byID[name] = c
	return nil
}

func (r *Registry) Get(name string) (Connector, bool) {
	key := normalizeName(name)
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[key]
	return c, ok
}

func (r *Registry) MustGet(name string) (Connector, error) {
	c, ok := r.Get(name)
	if !ok {
		return nil, ErrConnectorMissing
	}
	return c, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byID))
	for name := range r.byID {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Send(ctx context.Context, msg OutboundMessage) (DeliveryResult, error) {
	key := normalizeName(msg.Connector)
	if key == "" {
		return DeliveryResult{}, ErrInvalidConnector
	}
	c, err := r.MustGet(key)
	if err != nil {
		return DeliveryResult{}, err
	}
	msg.Connector = key
	return c.Send(ctx, msg)
}
