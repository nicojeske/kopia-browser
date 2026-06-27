// Package kopia is the read-only data layer: it talks to Garage S3 through the
// kopia Go library and returns plain Go values. It has no knowledge of HTTP or
// HTML, so handlers can be tested against a fake implementation. Every
// operation here is strictly read-only (see CLAUDE.md hard rules).
package kopia

import "time"

// SnapshotInfo is a UI-facing view of one kopia snapshot manifest, decoupled
// from kopia's internal types so templates never import kopia. The ugly Velero
// source path is deliberately omitted (see docs/KOPIA.md); friendly Tags are
// kept instead.
type SnapshotInfo struct {
	ID         string            // snapshot manifest id
	BackupName string            // Tags["backup"], the friendly Velero backup name
	Volume     string            // Tags["volume"], the Velero PVC volume name (empty if unset)
	StartTime  time.Time         // snapshot start
	EndTime    time.Time         // snapshot end
	TotalSize  int64             // total file bytes (Stats.TotalFileSize)
	FileCount  int64             // total files (Stats.TotalFileCount)
	Tags           map[string]string // full Velero tag set (backup, ns, pod, volume, ...)
	RetentionRoles []string          // deduplicated retention categories in canonical order (Latest, Hourly, Daily, Weekly, Monthly, Annual)
	Pinned         bool              // true when man.Pins is non-empty (e.g. velero-pin)
	ErrorCount     int32             // man.Stats.ErrorCount; non-zero means backup had errors
}

// DirEntry is a UI-facing view of one entry (file or directory) within a
// snapshot's directory tree. kopia fs.* types never escape this package.
type DirEntry struct {
	Name    string
	IsDir   bool
	Size    int64     // bytes; for directories, recursive total from kopia DirectorySummary (stored in manifest, no extra I/O)
	ModTime time.Time
}
