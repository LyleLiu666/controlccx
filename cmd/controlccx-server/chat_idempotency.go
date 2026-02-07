package main

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

type chatIdempotencyCache struct {
	mu         sync.Mutex
	entries    map[string]*chatIdempotencyEntry
	ttl        time.Duration
	maxEntries int
}

type chatIdempotencyEntry struct {
	createdAt time.Time
	done      bool
	reply     string
	err       string
	wait      chan struct{}
}

type chatIdempotencyLease struct {
	cache  *chatIdempotencyCache
	key    string
	leader bool
}

type chatIdempotencyResult struct {
	Reply string
	Error string
}

func newChatIdempotencyCache(ttl time.Duration, maxEntries int) *chatIdempotencyCache {
	if ttl <= 0 {
		ttl = 20 * time.Second
	}
	if maxEntries <= 0 {
		maxEntries = 2048
	}
	return &chatIdempotencyCache{
		entries:    make(map[string]*chatIdempotencyEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (c *chatIdempotencyCache) Acquire(ctx context.Context, key string) (chatIdempotencyLease, *chatIdempotencyResult, error) {
	if c == nil || key == "" {
		return chatIdempotencyLease{}, nil, nil
	}
	if ctx == nil {
		return chatIdempotencyLease{}, nil, errors.New("missing context")
	}

	now := time.Now()

	c.mu.Lock()
	c.cleanupLocked(now)
	if e, ok := c.entries[key]; ok {
		if e.done {
			res := &chatIdempotencyResult{Reply: e.reply, Error: e.err}
			c.mu.Unlock()
			return chatIdempotencyLease{}, res, nil
		}
		wait := e.wait
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return chatIdempotencyLease{}, nil, ctx.Err()
		case <-wait:
		}

		c.mu.Lock()
		defer c.mu.Unlock()
		e2, ok := c.entries[key]
		if ok && e2.done {
			res := &chatIdempotencyResult{Reply: e2.reply, Error: e2.err}
			return chatIdempotencyLease{}, res, nil
		}

		// Unexpected: treat as a new leader request.
		entry := &chatIdempotencyEntry{createdAt: now, wait: make(chan struct{})}
		c.entries[key] = entry
		return chatIdempotencyLease{cache: c, key: key, leader: true}, nil, nil
	}

	c.entries[key] = &chatIdempotencyEntry{createdAt: now, wait: make(chan struct{})}
	c.mu.Unlock()
	return chatIdempotencyLease{cache: c, key: key, leader: true}, nil, nil
}

func (l chatIdempotencyLease) Finish(reply string, err error) {
	if !l.leader || l.cache == nil || l.key == "" {
		return
	}

	l.cache.mu.Lock()
	defer l.cache.mu.Unlock()
	e, ok := l.cache.entries[l.key]
	if !ok || e.done {
		return
	}
	e.done = true
	e.reply = reply
	if err != nil {
		e.err = err.Error()
	} else {
		e.err = ""
	}
	close(e.wait)
}

func (c *chatIdempotencyCache) cleanupLocked(now time.Time) {
	if c == nil || len(c.entries) == 0 {
		return
	}

	cutoff := now.Add(-c.ttl)
	for k, e := range c.entries {
		if !e.done {
			continue
		}
		if e.createdAt.Before(cutoff) {
			delete(c.entries, k)
		}
	}

	if len(c.entries) <= c.maxEntries {
		return
	}

	type item struct {
		key string
		at  time.Time
	}
	var done []item
	for k, e := range c.entries {
		if !e.done {
			continue
		}
		done = append(done, item{key: k, at: e.createdAt})
	}
	sort.Slice(done, func(i, j int) bool { return done[i].at.Before(done[j].at) })

	excess := len(c.entries) - c.maxEntries
	for i := 0; i < len(done) && excess > 0; i++ {
		delete(c.entries, done[i].key)
		excess--
	}
}
