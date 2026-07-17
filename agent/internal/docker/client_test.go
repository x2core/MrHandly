package docker

import (
	"context"
	"errors"
	"testing"
)

func TestPing(t *testing.T) {
	d := newTestDaemon(t, true)
	if err := d.client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPingUnavailable(t *testing.T) {
	// A client pointed at a dead socket must report ErrUnavailable.
	c := New("/no/such/docker.sock", true)
	err := c.Ping(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestListContainers(t *testing.T) {
	d := newTestDaemon(t, true)
	cs, err := d.client.ListContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d containers, want 2", len(cs))
	}
	web := cs[0]
	if web.Name != "web" { // leading slash stripped
		t.Errorf("name = %q", web.Name)
	}
	if web.Image != "nginx:latest" || web.State != "running" {
		t.Errorf("container = %+v", web)
	}
	if !web.Writable {
		t.Error("expected writable=true for a writable client")
	}
	if cs[1].State != "exited" {
		t.Errorf("db state = %q", cs[1].State)
	}
}

func TestListContainersReadOnly(t *testing.T) {
	d := newTestDaemon(t, false)
	cs, err := d.client.ListContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Writable {
			t.Errorf("%s writable, want false on read-only client", c.Name)
		}
	}
}

func TestInspectContainer(t *testing.T) {
	d := newTestDaemon(t, true)
	c, err := d.client.InspectContainer(context.Background(), "abc123def456")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "web" {
		t.Errorf("name = %q", c.Name)
	}
	if c.Image != "nginx:latest" {
		t.Errorf("image = %q", c.Image)
	}
	if c.Created != 1700000000 { // 2023-11-14T22:13:20Z
		t.Errorf("created = %d", c.Created)
	}
}

func TestInspectNotFound(t *testing.T) {
	d := newTestDaemon(t, true)
	_, err := d.client.InspectContainer(context.Background(), "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestContainerActions(t *testing.T) {
	d := newTestDaemon(t, true)
	ctx := context.Background()
	if err := d.client.StartContainer(ctx, "abc123def456"); err != nil {
		t.Fatal(err)
	}
	if err := d.client.StopContainer(ctx, "abc123def456"); err != nil {
		t.Fatal(err)
	}
	if err := d.client.RestartContainer(ctx, "abc123def456"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /v1.43/containers/abc123def456/start",
		"POST /v1.43/containers/abc123def456/stop",
		"POST /v1.43/containers/abc123def456/restart",
	}
	got := d.recorded()
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("missing request %q in %v", w, got)
		}
	}
}

func TestActionErrorMapped(t *testing.T) {
	d := newTestDaemon(t, true)
	d.failStart = true
	if err := d.client.StartContainer(context.Background(), "abc123def456"); err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestListImages(t *testing.T) {
	d := newTestDaemon(t, true)
	imgs, err := d.client.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 {
		t.Fatalf("got %d images, want 2", len(imgs))
	}
	if imgs[0].Tags[0] != "nginx:latest" || imgs[0].Size != 142000000 {
		t.Errorf("image = %+v", imgs[0])
	}
}
