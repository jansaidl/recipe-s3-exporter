package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"recipe-s3-exporter/internal/models"
	"recipe-s3-exporter/internal/storage"
	"recipe-s3-exporter/internal/zerops"
)

// execute runs a single export end to end, recording status and progress.
func (p *Pool) execute(ctx context.Context, runID int64) {
	run, err := p.db.GetRun(ctx, runID)
	if err != nil {
		p.logf("run %d: load failed: %v", runID, err)
		return
	}
	if run.JobID == nil {
		p.fail(ctx, runID, 0, "run has no associated job")
		return
	}

	job, err := p.db.GetJob(ctx, *run.JobID)
	if err != nil {
		p.fail(ctx, runID, 0, fmt.Sprintf("load job: %v", err))
		return
	}

	if err := p.runExport(ctx, runID, job); err != nil {
		p.logf("run %d (job %q): %v", runID, job.Name, err)
		// fail() is invoked inside runExport for known terminal states; this
		// guards any unexpected error path.
		p.fail(ctx, runID, 0, err.Error())
	}
}

func (p *Pool) runExport(ctx context.Context, runID int64, job *models.ExportJob) error {
	// Resolve and decrypt the Zerops token.
	tok, err := p.db.GetToken(ctx, job.ZeropsTokenID)
	if err != nil {
		p.fail(ctx, runID, 0, fmt.Sprintf("load token: %v", err))
		return nil
	}
	plainToken, err := p.cipher.Decrypt(tok.TokenCiphertext)
	if err != nil {
		p.fail(ctx, runID, 0, fmt.Sprintf("decrypt token: %v", err))
		return nil
	}
	zc := zerops.New(p.zeropsAPI, plainToken, p.authScheme)

	// Find the latest backup matching the job's tag filter.
	backups, err := zc.ListBackups(ctx, job.ServiceStackID)
	if err != nil {
		p.fail(ctx, runID, 0, fmt.Sprintf("list backups: %v", err))
		return nil
	}
	backup := latestMatching(backups, job.TagFilter)
	if backup == nil {
		p.db.FinishRun(ctx, runID, models.StatusSkipped, 0, "no backup matched the tag filter")
		return nil
	}

	// Build the S3 store from the decrypted target config.
	store, err := p.buildStore(ctx, job.TargetID)
	if err != nil {
		p.fail(ctx, runID, 0, err.Error())
		return nil
	}
	key := store.Key(job.ServiceStackID, backup.Name)

	// Idempotency: skip if the object already exists.
	if exists, err := store.Exists(ctx, key); err == nil && exists {
		p.db.FinishRun(ctx, runID, models.StatusSkipped, backup.Size, "already exported (object exists)")
		return nil
	}

	if err := p.db.StartRun(ctx, runID, backup.Name, backup.Size, key); err != nil {
		return fmt.Errorf("start run: %w", err)
	}

	// Open a non-empty download stream, retrying with a fresh URL if the backup
	// archive is not yet ready (empty body). Returns a buffered reader with the
	// first byte already peeked, plus the counting reader for byte totals.
	body, buffered, cr, err := p.openBackupStream(ctx, zc, runID, job.ServiceStackID, backup)
	if err != nil {
		p.fail(ctx, runID, 0, err.Error())
		return nil
	}
	defer body.Close()

	// Stream to S3 with an unknown size (bounded-memory multipart). This is
	// robust to any mismatch between the metadata size and the actual stream
	// length.
	p.logf("run %d: uploading %s (metadata size=%d) -> %s", runID, backup.Name, backup.Size, key)
	if err := store.Upload(ctx, key, buffered, -1, "application/octet-stream"); err != nil {
		p.fail(ctx, runID, cr.total, fmt.Sprintf("upload: %v", err))
		return nil
	}

	if err := p.db.FinishRun(ctx, runID, models.StatusSuccess, cr.total, ""); err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	p.logf("run %d (job %q): exported %s (%d bytes) -> %s", runID, job.Name, backup.Name, cr.total, key)
	return nil
}

// openBackupStream requests a download URL and opens the byte stream, retrying
// with a fresh URL when the response is empty (the backup archive may not be
// ready immediately). It returns the underlying body (for the caller to close),
// a buffered reader with the first byte already verified, and the counting
// reader tracking transferred bytes.
func (p *Pool) openBackupStream(ctx context.Context, zc *zerops.Client, runID int64, serviceStackID string, backup *zerops.Backup) (io.ReadCloser, *bufio.Reader, *countingReader, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		url, err := zc.CreateDownloadURL(ctx, serviceStackID, backup.Name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("download url: %w", err)
		}

		body, contentLen, err := zc.Download(ctx, url)
		if err != nil {
			lastErr = fmt.Errorf("download: %w", err)
			p.logf("run %d: download attempt %d/%d failed: %v", runID, attempt, maxAttempts, err)
		} else {
			cr := newCountingReader(ctx, body, func(c context.Context, n int64) {
				_ = p.db.UpdateRunProgress(c, runID, n)
			})
			buffered := bufio.NewReaderSize(cr, 64*1024)
			if _, perr := buffered.Peek(1); perr == nil {
				p.logf("run %d: download ready on attempt %d (CL=%d)", runID, attempt, contentLen)
				return body, buffered, cr, nil
			} else if perr == io.EOF {
				body.Close()
				lastErr = fmt.Errorf("download returned 0 bytes for backup %q (Zerops reported size %d; the backup may not be ready yet)", backup.Name, backup.Size)
				p.logf("run %d: empty download on attempt %d/%d", runID, attempt, maxAttempts)
			} else {
				body.Close()
				return nil, nil, nil, fmt.Errorf("download read failed: %w", perr)
			}
		}

		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
	}
	return nil, nil, nil, lastErr
}

// buildStore decrypts a target's credentials and returns a ready S3 store.
func (p *Pool) buildStore(ctx context.Context, targetID int64) (*storage.Store, error) {
	t, err := p.db.GetTarget(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("load target: %w", err)
	}
	ak, err := p.cipher.Decrypt(t.AccessKeyCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt access key: %w", err)
	}
	sk, err := p.cipher.Decrypt(t.SecretKeyCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret key: %w", err)
	}
	return storage.New(storage.Config{
		Endpoint:     t.Endpoint,
		Region:       t.Region,
		Bucket:       t.Bucket,
		Prefix:       t.Prefix,
		AccessKey:    ak,
		SecretKey:    sk,
		UsePathStyle: t.UsePathStyle,
		UseSSL:       t.UseSSL,
	})
}

func (p *Pool) fail(ctx context.Context, runID, bytes int64, msg string) {
	_ = p.db.FinishRun(ctx, runID, models.StatusFailed, bytes, msg)
}

// latestMatching returns the newest backup carrying ALL of the required tags.
// Backups are named by date, so lexical sort descending yields newest first.
// An empty filter matches any backup.
func latestMatching(backups []zerops.Backup, required []string) *zerops.Backup {
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name > backups[j].Name })
	for i := range backups {
		if hasAllTags(backups[i].Tags, required) {
			return &backups[i]
		}
	}
	return nil
}

func hasAllTags(have, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, t := range have {
		set[t] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}
