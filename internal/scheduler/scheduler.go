// Package scheduler triggers export jobs on their cron schedules.
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"recipe-s3-exporter/internal/db"
	"recipe-s3-exporter/internal/worker"
)

// Scheduler maps enabled jobs to cron entries and enqueues runs on tick.
type Scheduler struct {
	db   *db.DB
	pool *worker.Pool

	mu   sync.Mutex
	cron *cron.Cron
}

// New builds a scheduler.
func New(store *db.DB, pool *worker.Pool) *Scheduler {
	return &Scheduler{db: store, pool: pool}
}

// Start performs an initial load. Call Reload after any job change.
func (s *Scheduler) Start(ctx context.Context) error {
	return s.Reload(ctx)
}

// Reload rebuilds all cron entries from the currently enabled jobs.
func (s *Scheduler) Reload(ctx context.Context) error {
	jobs, err := s.db.ListEnabledJobs(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil {
		s.cron.Stop()
	}
	c := cron.New()

	for _, j := range jobs {
		j := j
		if j.ScheduleCron == "" {
			continue
		}
		id, err := c.AddFunc(j.ScheduleCron, func() {
			bg := context.Background()
			if _, err := s.pool.Enqueue(bg, j.ID); err != nil {
				log.Printf("[scheduler] enqueue job %d failed: %v", j.ID, err)
				return
			}
			next := s.nextRun(j.ScheduleCron)
			_ = s.db.MarkJobRun(bg, j.ID, time.Now(), next)
		})
		if err != nil {
			log.Printf("[scheduler] invalid cron %q for job %q: %v", j.ScheduleCron, j.Name, err)
			continue
		}
		// Record the upcoming run time for display.
		if next := s.entryNext(c, id); next != nil {
			_ = s.db.SetJobNextRun(ctx, j.ID, next)
		}
	}

	c.Start()
	s.cron = c
	log.Printf("[scheduler] loaded %d job(s)", len(jobs))
	return nil
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
	}
}

func (s *Scheduler) entryNext(c *cron.Cron, id cron.EntryID) *time.Time {
	e := c.Entry(id)
	if e.Next.IsZero() {
		return nil
	}
	t := e.Next
	return &t
}

func (s *Scheduler) nextRun(spec string) *time.Time {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return nil
	}
	t := sched.Next(time.Now())
	return &t
}
