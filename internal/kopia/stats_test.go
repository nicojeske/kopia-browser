package kopia

import (
	"testing"
	"time"
)

func TestComputeNamespaceStats(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		nsName      string
		snaps       []SnapshotInfo
		wantVols    int
		wantSnaps   int
		wantSize    int64
		wantLastBkp time.Time
	}{
		{
			name:   "empty input",
			nsName: "empty",
			snaps:  nil,
		},
		{
			name:   "single volume single snapshot",
			nsName: "ns1",
			snaps: []SnapshotInfo{
				{Volume: "data-pvc", EndTime: t1, TotalSize: 100},
			},
			wantVols:    1,
			wantSnaps:   1,
			wantSize:    100,
			wantLastBkp: t1,
		},
		{
			name:   "single volume multiple snapshots — only latest counts for size",
			nsName: "ns2",
			snaps: []SnapshotInfo{
				{Volume: "data-pvc", EndTime: t0, TotalSize: 50},
				{Volume: "data-pvc", EndTime: t2, TotalSize: 200}, // latest
				{Volume: "data-pvc", EndTime: t1, TotalSize: 150},
			},
			wantVols:    1,
			wantSnaps:   3,
			wantSize:    200, // only the t2 snapshot
			wantLastBkp: t2,
		},
		{
			name:   "multi-volume sums latest-per-volume",
			nsName: "ns3",
			snaps: []SnapshotInfo{
				{Volume: "data-pvc", EndTime: t2, TotalSize: 300},
				{Volume: "data-pvc", EndTime: t0, TotalSize: 100}, // older, not counted
				{Volume: "config-pvc", EndTime: t1, TotalSize: 50},
			},
			wantVols:    2,
			wantSnaps:   3,
			wantSize:    350, // 300 + 50
			wantLastBkp: t2,
		},
		{
			name:   "untagged volume (empty string) treated as one group",
			nsName: "ns4",
			snaps: []SnapshotInfo{
				{Volume: "", EndTime: t1, TotalSize: 80},
				{Volume: "", EndTime: t2, TotalSize: 90},
			},
			wantVols:    1,
			wantSnaps:   2,
			wantSize:    90, // latest
			wantLastBkp: t2,
		},
		{
			name:   "mixed tagged and untagged volumes",
			nsName: "ns5",
			snaps: []SnapshotInfo{
				{Volume: "data-pvc", EndTime: t2, TotalSize: 500},
				{Volume: "", EndTime: t1, TotalSize: 20},
			},
			wantVols:    2,
			wantSnaps:   2,
			wantSize:    520,
			wantLastBkp: t2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeNamespaceStats(tc.nsName, tc.snaps)

			if got.Name != tc.nsName {
				t.Errorf("Name = %q, want %q", got.Name, tc.nsName)
			}
			if got.Volumes != tc.wantVols {
				t.Errorf("Volumes = %d, want %d", got.Volumes, tc.wantVols)
			}
			if got.Snapshots != tc.wantSnaps {
				t.Errorf("Snapshots = %d, want %d", got.Snapshots, tc.wantSnaps)
			}
			if got.SizeBytes != tc.wantSize {
				t.Errorf("SizeBytes = %d, want %d", got.SizeBytes, tc.wantSize)
			}
			if !got.LastBackup.Equal(tc.wantLastBkp) {
				t.Errorf("LastBackup = %v, want %v", got.LastBackup, tc.wantLastBkp)
			}
		})
	}
}
