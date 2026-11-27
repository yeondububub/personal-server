package monitor

import (
	"testing"
	"time"

	"personal-server/internal/config"
	"personal-server/internal/files"
)

func TestSnapshotReturnsHostAndDiskStats(t *testing.T) {
	dir := t.TempDir()
	jail := files.NewJail([]config.MountRoot{
		{ID: "data", Name: "Data", Path: dir},
	})
	svc := NewService(jail, time.Now().Add(-10*time.Second))

	snap := svc.Snapshot("private")
	if snap.AccessTier != "private" {
		t.Fatalf("expected accessTier private, got %q", snap.AccessTier)
	}
	if snap.CPUCores <= 0 || len(snap.CPUPerCore) == 0 {
		t.Fatalf("expected cpuCores > 0 and cpuPerCore non-empty, got cores=%d perCore=%v", snap.CPUCores, snap.CPUPerCore)
	}
	if snap.MemTotalBytes == 0 {
		t.Fatalf("expected memTotalBytes > 0, got %d", snap.MemTotalBytes)
	}
	if len(snap.Disks) != 1 || snap.Disks[0].ID != "data" || snap.Disks[0].TotalBytes == 0 {
		t.Fatalf("expected valid disk stat for data root, got %+v", snap.Disks)
	}
}
