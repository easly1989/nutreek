//go:build demo

package main

import (
	"bytes"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	e2eCSRF     = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
	e2eAddItem  = regexp.MustCompile(`/meals/(\d+)/items/new`)
	e2eItem     = regexp.MustCompile(`action="/meal-items/\d+/delete`)
	e2eFoodEdit = regexp.MustCompile(`/ingredients/(\d+)/edit`)
)

// The demo binary end to end (#188): built with -tags demo, started on an
// empty database, entered from the home's form, a food added to the seeded week.
func TestDemoBinaryEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "nutreek")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-tags", "demo", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build -tags demo: %v\n%s", err, out)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	var logs bytes.Buffer
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"NUTREEK_ADDR="+addr,
		"NUTREEK_DB_PATH="+filepath.Join(dir, "demo.db"),
		"NUTREEK_ENV=dev",
		"NUTREEK_OFF_SYNC_ENABLED=false",
		"NUTREEK_RECONCILE_ENABLED=false",
	)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-exited // releases the database before TempDir is removed
		if t.Failed() {
			t.Logf("server log:\n%s", logs.String())
		}
	})

	base := "http://" + addr
	jar, _ := cookiejar.New(nil)
	c := &http.Client{
		Jar:           jar,
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	get := func(path string) (int, string) {
		t.Helper()
		resp, err := c.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	// post sends a form as a browser does: the CSRF token of page, and Origin.
	post := func(page, path string, form url.Values) (int, string) {
		t.Helper()
		m := e2eCSRF.FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("POST %s: no CSRF token on the page", path)
		}
		form.Set("csrf_token", html.UnescapeString(m[1]))
		req, _ := http.NewRequest(http.MethodPost, base+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", base)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Location")
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := c.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("/healthz not ready after 30s: %v", err)
		}
		select {
		case err := <-exited:
			exited <- err // for the cleanup
			t.Fatalf("the server exited at startup: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}

	code, entry := get("/")
	if code != http.StatusOK || !strings.Contains(entry, `action="/demo/session"`) {
		t.Fatalf("GET /: %d, no demo entry", code)
	}
	if code, loc := post(entry, "/demo/session", url.Values{}); code != http.StatusSeeOther || loc != "/" {
		t.Fatalf("POST /demo/session: %d to %q", code, loc)
	}

	_, week := get("/")
	meal := e2eAddItem.FindStringSubmatch(week)
	items := len(e2eItem.FindAllString(week, -1))
	if !strings.Contains(week, `id="demo-band"`) || meal == nil || items == 0 {
		t.Fatalf("the week after entering: band %v, meals %v, %d items", strings.Contains(week, `id="demo-band"`), meal != nil, items)
	}

	_, foods := get("/ingredients")
	food := e2eFoodEdit.FindStringSubmatch(foods)
	if food == nil {
		t.Fatal("GET /ingredients lists no food")
	}
	form := url.Values{"kind": {"ingredient"}, "ref_id": {food[1]}, "quantity": {"100"}}
	if code, _ := post(week, "/meals/"+meal[1]+"/items", form); code != http.StatusSeeOther {
		t.Fatalf("adding a food: %d", code)
	}
	if _, after := get("/"); len(e2eItem.FindAllString(after, -1)) != items+1 {
		t.Errorf("the week has %d items after adding a food, want %d", len(e2eItem.FindAllString(after, -1)), items+1)
	}
}
