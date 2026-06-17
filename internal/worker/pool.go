// Package worker runs export jobs concurrently: it resolves the latest matching
// backup from Zerops and streams it into the configured S3 target while
// recording live progress.
package worker

import (
	"context"
	"log"
	"sync"

	"recipe-s3-exporter/internal/crypto"
	"recipe-s3-exporter/internal/db"
)

// Pool is a fixed-size set of export workers fed by a buffered queue.
type Pool struct {
	db        *db.DB
	cipher    *crypto.Cipher
	zeropsAPI string

	queue chan int64 // run IDs to execute
	wg    sync.WaitGroup
}

// NewPool builds a worker pool. zeropsAPI may be empty to use the default.
func NewPool(store *db.DB, cipher *crypto.Cipher, zeropsAPI string, workers int) *Pool {
	if workers < 1 {
		workers = 1
	}
	return &Pool{
		db:        store,
		cipher:    cipher,
		zeropsAPI: zeropsAPI,
		queue:     make(chan int64, 256),
	}
}

// Start launches the worker goroutines.
func (p *Pool) Start(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go func(id int) {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case runID, ok := <-p.queue:
					if !ok {
						return
					}
					p.execute(ctx, runID)
				}
			}
		}(i)
	}
}

// Enqueue creates a pending run for the job and schedules it. It returns the new
// run id. Sending happens in a goroutine so callers never block on a full queue.
func (p *Pool) Enqueue(ctx context.Context, jobID int64) (int64, error) {
	runID, err := p.db.CreateRun(ctx, &jobID)
	if err != nil {
		return 0, err
	}
	go func() { p.queue <- runID }()
	return runID, nil
}

// Wait blocks until all workers have stopped (after the context is cancelled).
func (p *Pool) Wait() { p.wg.Wait() }

func (p *Pool) logf(format string, args ...any) { log.Printf("[worker] "+format, args...) }
