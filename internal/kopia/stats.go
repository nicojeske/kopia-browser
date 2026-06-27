package kopia

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// NamespaceStats summarises the backup state of one namespace.
// SizeBytes is the sum of each volume's newest snapshot's TotalSize — it is
// a logical (non-deduplication-adjusted) approximation; see docs/KOPIA.md.
type NamespaceStats struct {
	Name       string
	Volumes    int       // distinct volume count
	Snapshots  int       // total snapshot count
	SizeBytes  int64     // latest-per-volume TotalSize sum (logical; see note)
	LastBackup time.Time // newest EndTime across all snapshots in this namespace
}

// StatsSnapshot is an immutable, point-in-time copy of computed dashboard
// stats. Once obtained from StatsCache.Get it is safe to read without locks.
type StatsSnapshot struct {
	Namespaces     []NamespaceStats // sorted by SizeBytes descending
	TotalSize      int64
	TotalSnapshots int
	NamespaceCount int
	MaxSize        int64     // largest SizeBytes value (used for bar scaling)
	UpdatedAt      time.Time
	Ready          bool // false until the first successful refresh completes
}

// computeNamespaceStats derives NamespaceStats for one namespace from its
// snapshot list. It is pure (no I/O) and directly unit-testable.
// Size = sum of each distinct volume's newest-EndTime snapshot TotalSize.
func computeNamespaceStats(name string, snaps []SnapshotInfo) NamespaceStats {
	if len(snaps) == 0 {
		return NamespaceStats{Name: name}
	}

	type volState struct {
		latestEnd time.Time
		size      int64
	}
	byVol := map[string]*volState{}
	var lastBackup time.Time

	for _, s := range snaps {
		if s.EndTime.After(lastBackup) {
			lastBackup = s.EndTime
		}
		vs, ok := byVol[s.Volume]
		if !ok {
			vs = &volState{}
			byVol[s.Volume] = vs
		}
		if s.EndTime.After(vs.latestEnd) {
			vs.latestEnd = s.EndTime
			vs.size = s.TotalSize
		}
	}

	var totalSize int64
	for _, vs := range byVol {
		totalSize += vs.size
	}

	return NamespaceStats{
		Name:       name,
		Volumes:    len(byVol),
		Snapshots:  len(snaps),
		SizeBytes:  totalSize,
		LastBackup: lastBackup,
	}
}

// StatsCache holds the latest computed StatsSnapshot and refreshes it
// periodically by calling the Manager. Safe for concurrent use.
type StatsCache struct {
	mgr         *Manager
	interval    time.Duration
	persistPath string

	mu  sync.RWMutex
	cur StatsSnapshot
}

// NewStatsCache creates a StatsCache that refreshes on the given interval.
// persistPath is the file used to save/restore the snapshot across restarts;
// pass "" to disable persistence. Call Run(ctx) in a goroutine to start
// background refreshes.
func NewStatsCache(mgr *Manager, interval time.Duration, persistPath string) *StatsCache {
	c := &StatsCache{mgr: mgr, interval: interval, persistPath: persistPath}
	if persistPath != "" {
		c.load()
	}
	return c
}

// Get returns a copy of the latest StatsSnapshot. Safe for concurrent calls.
func (c *StatsCache) Get() StatsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cur
}

// Run refreshes immediately (unless cache is still fresh), then repeats every interval.
// Exits when ctx is cancelled (e.g. on server shutdown).
func (c *StatsCache) Run(ctx context.Context) {
	c.mu.RLock()
	age := time.Since(c.cur.UpdatedAt)
	c.mu.RUnlock()
	if c.cur.UpdatedAt.IsZero() || age >= c.interval {
		c.refresh(ctx)
	} else {
		slog.Info("stats: skipping initial refresh, cache is fresh", "age", age.Round(time.Second), "interval", c.interval)
	}

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refresh(ctx)
		}
	}
}

// refresh rebuilds the snapshot from the Manager. Per-namespace errors are
// logged and skipped so one unavailable repo doesn't blank the whole dashboard.
func (c *StatsCache) refresh(ctx context.Context) {
	if ctx.Err() != nil {
		return // already cancelled; don't bother
	}

	start := time.Now()

	namespaces, err := c.mgr.ListNamespaces(ctx)
	if err != nil {
		slog.Error("stats: list namespaces failed", "err", err)
		return
	}

	slog.Info("stats: refresh starting", "namespaces", len(namespaces))

	nsList := make([]NamespaceStats, 0, len(namespaces))
	var totalSize int64
	var totalSnaps int

	for i, ns := range namespaces {
		slog.Info("stats: processing namespace", "i", i+1, "n", len(namespaces), "ns", ns)
		snaps, err := c.mgr.ListSnapshots(ctx, ns)
		if err != nil {
			slog.Warn("stats: list snapshots failed, skipping", "ns", ns, "err", err)
			continue
		}
		slog.Debug("stats: namespace done", "ns", ns, "snapshots", len(snaps))
		st := computeNamespaceStats(ns, snaps)
		nsList = append(nsList, st)
		totalSize += st.SizeBytes
		totalSnaps += st.Snapshots
	}

	// Sort descending by SizeBytes; largest namespace drives MaxSize for bars.
	sort.Slice(nsList, func(i, j int) bool {
		return nsList[i].SizeBytes > nsList[j].SizeBytes
	})

	var maxSize int64
	if len(nsList) > 0 {
		maxSize = nsList[0].SizeBytes
	}

	snap := StatsSnapshot{
		Namespaces:     nsList,
		TotalSize:      totalSize,
		TotalSnapshots: totalSnaps,
		NamespaceCount: len(namespaces),
		MaxSize:        maxSize,
		UpdatedAt:      time.Now(),
		Ready:          true,
	}

	c.mu.Lock()
	c.cur = snap
	c.mu.Unlock()

	slog.Info("stats: refresh done", "namespaces", len(namespaces), "snapshots", totalSnaps, "duration", time.Since(start).Round(time.Millisecond))

	if c.persistPath != "" {
		c.save(snap)
	}
}

// load reads a persisted StatsSnapshot from disk. Errors are logged and ignored
// so a missing or corrupt file never prevents startup.
func (c *StatsCache) load() {
	data, err := os.ReadFile(c.persistPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("stats: load cache file failed", "path", c.persistPath, "err", err)
		}
		return
	}
	var snap StatsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		slog.Warn("stats: cache file corrupt, ignoring", "path", c.persistPath, "err", err)
		return
	}
	c.mu.Lock()
	c.cur = snap
	c.mu.Unlock()
	slog.Info("stats: loaded cached snapshot", "path", c.persistPath, "age", time.Since(snap.UpdatedAt).Round(time.Second))
}

// save writes snap to persistPath atomically (write-then-rename).
func (c *StatsCache) save(snap StatsSnapshot) {
	data, err := json.Marshal(snap)
	if err != nil {
		slog.Error("stats: save marshal failed", "err", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.persistPath), 0o755); err != nil {
		slog.Error("stats: save mkdir failed", "path", c.persistPath, "err", err)
		return
	}
	tmp := c.persistPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Error("stats: save write failed", "path", tmp, "err", err)
		return
	}
	if err := os.Rename(tmp, c.persistPath); err != nil {
		slog.Error("stats: save rename failed", "src", tmp, "dst", c.persistPath, "err", err)
	}
}
