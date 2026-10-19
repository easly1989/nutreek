package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const list = `{"stable": {"image": "ghcr.io/easly1989/nutreek:latest", "sha": "8fad9493d984372b1eedf45e1450e7812e58374b", "published": "2026-10-14T09:12:00Z", "version": "v2.1.0"}}`

// memSettings is Settings in memory.
type memSettings map[string]string

func (m memSettings) Get(_ context.Context, k string) (string, bool, error) {
	v, ok := m[k]
	return v, ok, nil
}

func (m memSettings) Set(_ context.Context, k, v string) error {
	m[k] = v
	return nil
}

// newService checks against a server answering body, and counts its requests.
func newService(t *testing.T, version string, allowed bool, body string) (*Service, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	s := New(memSettings{}, version, allowed)
	s.SetURL(srv.URL)
	return s, &hits
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{
		"v2.0.0": true, "v2.1.0-rc1": true,
		"dev": false, "latest-1b97df6": false, "2.0.0": false, "v2.0": false,
	} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v2.1.0", "v2.0.0", true}, {"v2.0.10", "v2.0.9", true}, {"v3.0.0", "v2.9.9", true},
		{"v2.0.0", "v2.0.0", false}, {"v2.0.0", "v2.1.0", false}, {"v2.1.0", "dev", false},
		{"v2.1.0", "v2.1.0-rc1", true}, {"v2.1.0-rc1", "v2.1.0", false}, {"v2.1.0+build.5", "v2.1.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSeesANewerRelease(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t, "v2.0.0", true, list)
	st, err := s.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || st.Latest != "v2.1.0" || !st.Active() || st.Err != nil || st.CheckedAt.IsZero() || st.Published.IsZero() {
		t.Fatalf("status = %+v", st)
	}
	if st.ReleaseURL != "https://github.com/easly1989/nutreek/releases/tag/v2.1.0" {
		t.Errorf("release url = %q", st.ReleaseURL)
	}

	for _, v := range []string{"v2.1.0", "v2.2.0"} {
		s, _ := newService(t, v, true, list)
		if st, _ := s.Check(ctx); st.Available || st.Latest != "v2.1.0" {
			t.Errorf("%s: %+v, want up to date", v, st)
		}
	}
}

func TestOffMeansNoRequest(t *testing.T) {
	ctx := context.Background()

	// NUTREEK_UPDATE_CHECK=false.
	s, hits := newService(t, "v2.0.0", false, list)
	if st, _ := s.Check(ctx); *hits != 0 || st.Active() || st.Available {
		t.Errorf("turned off for the installation, still asked: hits=%d %+v", *hits, st)
	}

	// Builds that are no release.
	for _, v := range []string{"dev", "latest-1b97df6"} {
		s, hits := newService(t, v, true, list)
		if st, _ := s.Check(ctx); *hits != 0 || st.Release {
			t.Errorf("%s asked: hits=%d %+v", v, *hits, st)
		}
	}

	// The owner turns it off, and back on.
	on, hits := newService(t, "v2.0.0", true, list)
	st, err := on.SetEnabled(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || *hits != 0 {
		t.Errorf("after turning off: hits=%d %+v", *hits, st)
	}
	if st, _ := on.Check(ctx); *hits != 0 || st.Available {
		t.Errorf("turned off, a check still asked: hits=%d %+v", *hits, st)
	}
	if st, _ = on.SetEnabled(ctx, true); !st.Enabled || *hits != 1 || !st.Available {
		t.Errorf("turning on checks at once: hits=%d %+v", *hits, st)
	}
}

func TestFailedCheckIsReported(t *testing.T) {
	ctx := context.Background()
	unreachable, _ := newService(t, "v2.0.0", true, list)
	unreachable.SetURL("http://127.0.0.1:1/versions.json")
	for name, s := range map[string]*Service{
		"unreachable": unreachable,
		"bad json":    func() *Service { s, _ := newService(t, "v2.0.0", true, "not json"); return s }(),
		"no release":  func() *Service { s, _ := newService(t, "v2.0.0", true, `{"stable": {}}`); return s }(),
	} {
		st, err := s.Check(ctx)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st.Err == nil || st.Available || st.CheckedAt.IsZero() {
			t.Errorf("%s: status = %+v", name, st)
		}
	}
}
