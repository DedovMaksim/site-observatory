package scheduler

import (
	"context"
	"example.com/site-observatory/internal/monitor"
	"example.com/site-observatory/internal/storage"
	"log"
	"sync"
	"time"
)

type Scheduler struct {
	DB      *storage.DB
	Checker monitor.Checker
	mu      sync.Mutex
	last    map[int64]time.Time
	sem     chan struct{}
}

func New(db *storage.DB, c monitor.Checker) *Scheduler {
	return &Scheduler{DB: db, Checker: c, last: map[int64]time.Time{}, sem: make(chan struct{}, 10)}
}
func (s *Scheduler) Run(ctx context.Context) {
	s.dispatch(ctx)
	tick := time.NewTicker(5 * time.Second)
	cleanup := time.NewTicker(24 * time.Hour)
	defer tick.Stop()
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.dispatch(ctx)
		case <-cleanup.C:
			if e := s.DB.Prune(ctx); e != nil {
				log.Printf("cleanup: %v", e)
			}
		}
	}
}
func (s *Scheduler) dispatch(ctx context.Context) {
	list, e := s.DB.Monitors()
	if e != nil {
		log.Printf("scheduler list: %v", e)
		return
	}
	now := time.Now()
	for _, m := range list {
		if !m.Enabled {
			continue
		}
		s.mu.Lock()
		last := s.last[m.ID]
		if !last.IsZero() && now.Sub(last) < time.Duration(m.Interval)*time.Second {
			s.mu.Unlock()
			continue
		}
		s.last[m.ID] = now
		s.mu.Unlock()
		select {
		case s.sem <- struct{}{}:
			go func(m storage.Monitor) {
				defer func() { <-s.sem }()
				result := s.Checker.Check(ctx, m.URL, m.Timeout)
				if ctx.Err() != nil {
					return
				}
				if e := s.DB.Record(ctx, m.ID, result); e != nil {
					log.Printf("check record %d: %v", m.ID, e)
				}
			}(m)
		default:
			s.mu.Lock()
			delete(s.last, m.ID)
			s.mu.Unlock()
		}
	}
}
