package procfs

import "testing"

func findProc(ps []ProcStat, pid int) (ProcStat, bool) {
	for _, p := range ps {
		if p.PID == pid {
			return p, true
		}
	}
	return ProcStat{}, false
}

func TestProcesses(t *testing.T) {
	r := New(fixtureRoot)
	ps, err := r.Processes()
	if err != nil {
		t.Fatalf("Processes: %v", err)
	}

	nginx, ok := findProc(ps, 101)
	if !ok {
		t.Fatal("pid 101 missing")
	}
	if nginx.Comm != "nginx" || nginx.State != "S" || nginx.PPID != 1 {
		t.Errorf("101 = %+v", nginx)
	}
	if nginx.Jiffies != 150 { // utime 100 + stime 50
		t.Errorf("101 jiffies = %d, want 150", nginx.Jiffies)
	}
	if nginx.Threads != 4 {
		t.Errorf("101 threads = %d", nginx.Threads)
	}
	if nginx.RSS != 2000*pageSize {
		t.Errorf("101 rss = %d, want %d", nginx.RSS, 2000*pageSize)
	}
	if nginx.Cmdline != "nginx: master process /usr/sbin/nginx" {
		t.Errorf("101 cmdline = %q", nginx.Cmdline)
	}

	// pid 202 has a comm with spaces and parentheses — split on the last ')'.
	app, ok := findProc(ps, 202)
	if !ok {
		t.Fatal("pid 202 missing")
	}
	if app.Comm != "app (v2)" {
		t.Errorf("202 comm = %q, want 'app (v2)'", app.Comm)
	}
	if app.State != "R" || app.PPID != 100 {
		t.Errorf("202 = %+v", app)
	}
	if app.Jiffies != 300 { // 200 + 100
		t.Errorf("202 jiffies = %d, want 300", app.Jiffies)
	}
}
