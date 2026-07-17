package procfs

import "testing"

const fixtureRoot = "../../testdata/host-a"

func TestStat(t *testing.T) {
	r := New(fixtureRoot)
	var s Stat
	if err := r.Stat(&s); err != nil {
		t.Fatalf("Stat: %v", err)
	}

	wantTotal := CPUTimes{User: 100, Nice: 20, System: 50, Idle: 1000, Iowait: 30, IRQ: 0, SoftIRQ: 5, Steal: 0}
	if s.Total != wantTotal {
		t.Errorf("Total = %+v, want %+v", s.Total, wantTotal)
	}
	if len(s.PerCPU) != 2 {
		t.Fatalf("PerCPU len = %d, want 2", len(s.PerCPU))
	}
	if got, want := s.PerCPU[1].SoftIRQ, uint64(3); got != want {
		t.Errorf("PerCPU[1].SoftIRQ = %d, want %d", got, want)
	}
	if got, want := s.Total.Total(), uint64(1205); got != want {
		t.Errorf("Total.Total() = %d, want %d", got, want)
	}
	if got, want := s.Total.Idleness(), uint64(1030); got != want {
		t.Errorf("Total.Idleness() = %d, want %d", got, want)
	}
}

// TestStatReuse verifies PerCPU's backing array is reused across reads and the
// slice length is reset each time (no stale cores from a previous sample).
func TestStatReuse(t *testing.T) {
	r := New(fixtureRoot)
	var s Stat
	if err := r.Stat(&s); err != nil {
		t.Fatal(err)
	}
	first := &s.PerCPU[0]
	if err := r.Stat(&s); err != nil {
		t.Fatal(err)
	}
	if len(s.PerCPU) != 2 {
		t.Fatalf("PerCPU len after reuse = %d, want 2", len(s.PerCPU))
	}
	if &s.PerCPU[0] != first {
		t.Error("PerCPU backing array was reallocated; expected reuse")
	}
}

func TestMemInfo(t *testing.T) {
	r := New(fixtureRoot)
	m, err := r.MemInfo()
	if err != nil {
		t.Fatalf("MemInfo: %v", err)
	}
	if got, want := m.Total, uint64(16384000)*1024; got != want {
		t.Errorf("Total = %d, want %d", got, want)
	}
	if got, want := m.Available, uint64(10240000)*1024; got != want {
		t.Errorf("Available = %d, want %d", got, want)
	}
	if got, want := m.SwapTotal, uint64(2097152)*1024; got != want {
		t.Errorf("SwapTotal = %d, want %d", got, want)
	}
}

func TestNetDev(t *testing.T) {
	r := New(fixtureRoot)
	nd, err := r.NetDev()
	if err != nil {
		t.Fatalf("NetDev: %v", err)
	}
	if _, ok := nd["lo"]; ok {
		t.Error("loopback should be excluded")
	}
	eth0, ok := nd["eth0"]
	if !ok {
		t.Fatal("eth0 missing")
	}
	if eth0.RxBytes != 987654321 || eth0.RxPackets != 654321 {
		t.Errorf("eth0 rx = %d/%d", eth0.RxBytes, eth0.RxPackets)
	}
	if eth0.TxBytes != 123456789 || eth0.TxPackets != 234567 {
		t.Errorf("eth0 tx = %d/%d", eth0.TxBytes, eth0.TxPackets)
	}
	if wg0 := nd["wg0"]; wg0.RxBytes != 4096000 {
		t.Errorf("wg0 rx = %d, want 4096000", wg0.RxBytes)
	}
}

func TestLoadAvg(t *testing.T) {
	r := New(fixtureRoot)
	la, err := r.LoadAvg()
	if err != nil {
		t.Fatalf("LoadAvg: %v", err)
	}
	if la.One != 0.52 || la.Five != 0.48 || la.Fifteen != 0.40 {
		t.Errorf("load = %v", la)
	}
}

func TestUptime(t *testing.T) {
	r := New(fixtureRoot)
	up, err := r.Uptime()
	if err != nil {
		t.Fatalf("Uptime: %v", err)
	}
	if up != 123456.78 {
		t.Errorf("uptime = %v, want 123456.78", up)
	}
}

func TestDiskStats(t *testing.T) {
	r := New(fixtureRoot)
	ds, err := r.DiskStats()
	if err != nil {
		t.Fatalf("DiskStats: %v", err)
	}
	for _, virt := range []string{"ram0", "loop0"} {
		if _, ok := ds[virt]; ok {
			t.Errorf("virtual device %q should be excluded", virt)
		}
	}
	nvme, ok := ds["nvme0n1"]
	if !ok {
		t.Fatal("nvme0n1 missing")
	}
	if nvme.Reads != 100000 || nvme.ReadSectors != 8000000 {
		t.Errorf("nvme reads = %d/%d", nvme.Reads, nvme.ReadSectors)
	}
	if nvme.Writes != 80000 || nvme.WriteSectors != 4000000 {
		t.Errorf("nvme writes = %d/%d", nvme.Writes, nvme.WriteSectors)
	}
	if _, ok := ds["sda"]; !ok {
		t.Error("sda missing")
	}
}

func TestMissingRoot(t *testing.T) {
	r := New("/no/such/root")
	var s Stat
	if err := r.Stat(&s); err == nil {
		t.Error("expected error for missing /proc/stat")
	}
}
