package internal

import (
	"net/url"
	"testing"
)

func TestGetBaseURLFromServersSelectsEveryServer(t *testing.T) {
	servers := map[string]Server{}
	for _, id := range []string{"a", "b", "c"} {
		u, err := url.Parse("http://" + id + ".example.com")
		if err != nil {
			t.Fatal(err)
		}

		servers[id] = Server{URL: u}
	}

	us := &UpstreamSetting{servers: servers}
	seen := map[string]bool{}

	for range 1000 {
		u, id, err := us.getBaseURLFromServers("test", nil)
		if err != nil {
			t.Fatal(err)
		}

		if u.Host != id+".example.com" {
			t.Fatalf("server %s returned mismatched URL %s", id, u)
		}

		seen[id] = true
	}

	if len(seen) != len(servers) {
		t.Fatalf("expected all %d servers to be selected, got %v", len(servers), seen)
	}

	_, id, err := us.getBaseURLFromServers("test", []string{"c"})
	if err != nil || id != "c" {
		t.Fatalf("expected pinned server c, got %q (err: %v)", id, err)
	}
}
