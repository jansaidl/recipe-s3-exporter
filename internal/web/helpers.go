package web

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"recipe-s3-exporter/internal/models"
)

// humanBytes renders a byte count in human-readable units.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// fmtTime formats a timestamp for display, or "—" if zero.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// fmtTimePtr formats an optional timestamp.
func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return fmtTime(*t)
}

// joinTags renders a tag slice as a comma-separated string.
func joinTags(tags []string) string {
	if len(tags) == 0 {
		return "any"
	}
	return strings.Join(tags, ", ")
}

// joinTagsRaw renders tags comma-separated, empty string when none (for inputs).
func joinTagsRaw(tags []string) string {
	return strings.Join(tags, ", ")
}

// statusClass maps a run status to a Tailwind badge class.
func statusClass(s models.RunStatus) string {
	switch s {
	case models.StatusSuccess:
		return "bg-emerald-100 text-emerald-800"
	case models.StatusFailed:
		return "bg-red-100 text-red-800"
	case models.StatusRunning:
		return "bg-blue-100 text-blue-800"
	case models.StatusPending:
		return "bg-amber-100 text-amber-800"
	case models.StatusSkipped:
		return "bg-slate-100 text-slate-600"
	default:
		return "bg-slate-100 text-slate-600"
	}
}

// isActive reports whether a run is still in flight.
func isActive(s models.RunStatus) bool {
	return s == models.StatusPending || s == models.StatusRunning
}

// formatID builds a path from a printf format and an id (used in templates).
func formatID(format string, id int64) string {
	return fmt.Sprintf(format, id)
}

// downloadHref builds the presigned-download redirect URL for an object.
func downloadHref(targetID int64, key string) string {
	return fmt.Sprintf("/explore/download?target_id=%d&key=%s", targetID, url.QueryEscape(key))
}
