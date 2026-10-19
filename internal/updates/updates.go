// Package updates tells the owner that a newer Nutreek has been published
// (#167). Once a day the server reads versions.json on the repository's
// site-data branch, written by the Release workflow, and compares it with the
// version it was built as. It sends nothing: it is a plain request for a
// public file, made by the server so that GitHub sees one address rather than
// every device that opens the app.
//
// Updating stays outside the app. Replacing the container from inside would
// need the Docker socket, and whoever holds that socket holds the host.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultURL is where the published versions are listed.
const DefaultURL = "https://raw.githubusercontent.com/easly1989/nutreek/site-data/versions.json"

const releases = "https://github.com/easly1989/nutreek/releases/tag/"

// settingKey stores whether the owner left the check on ("1") or turned it off ("0").
const settingKey = "update_check"

// Settings stores the owner's choice; db.InstanceSettings is one.
type Settings interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
}

// Status is what the owner sees: the build, the newest published release,
// and whether the check runs at all.
type Status struct {
	Enabled bool   // the owner's choice
	Allowed bool   // the installation's: NUTREEK_UPDATE_CHECK
	Release bool   // the build is a release (vX.Y.Z); dev and latest-<sha> builds never check
	Current string // the version this server was built as
	// Set once a check has run.
	CheckedAt time.Time
	Err       error
	// Set once a check has succeeded.
	Latest     string
	Published  time.Time
	Available  bool
	ReleaseURL string
}

// Active reports whether the server checks: allowed, enabled and a release.
func (s Status) Active() bool { return s.Allowed && s.Enabled && s.Release }

// Service runs the check and remembers its last answer, in memory.
type Service struct {
	settings Settings
	current  string
	allowed  bool
	url      string
	client   *http.Client
	now      func() time.Time

	mu   sync.Mutex
	last *result
}

type result struct {
	latest    string
	published time.Time
	checkedAt time.Time
	err       error
}

// New prepares the check for a build of version. allowed is false when
// NUTREEK_UPDATE_CHECK turns it off for the whole installation.
func New(settings Settings, version string, allowed bool) *Service {
	return &Service{
		settings: settings,
		current:  version,
		allowed:  allowed,
		url:      DefaultURL,
		client:   &http.Client{Timeout: 15 * time.Second},
		now:      time.Now,
	}
}

// SetURL points the check elsewhere (tests).
func (s *Service) SetURL(u string) { s.url = u }

// IsRelease reports whether version is a release ("v1.2.3"), the only builds
// that follow the published versions.
func IsRelease(version string) bool { return parseSemver(version) != nil }

func (s *Service) enabled(ctx context.Context) (bool, error) {
	v, ok, err := s.settings.Get(ctx, settingKey)
	if err != nil || !ok {
		return true, err // on unless the owner turned it off
	}
	return v != "0", nil
}

// Status returns the last answer without asking again.
func (s *Service) Status(ctx context.Context) (Status, error) {
	on, err := s.enabled(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Enabled: on, Allowed: s.allowed, Release: IsRelease(s.current), Current: s.current}
	s.mu.Lock()
	last := s.last
	s.mu.Unlock()
	if last == nil || !st.Active() {
		return st, nil
	}
	st.CheckedAt, st.Err = last.checkedAt, last.err
	if last.err == nil {
		st.Latest, st.Published = last.latest, last.published
		st.Available = newer(last.latest, s.current)
		st.ReleaseURL = releases + last.latest
	}
	return st, nil
}

// SetEnabled records the owner's choice; turning the check on runs it.
func (s *Service) SetEnabled(ctx context.Context, on bool) (Status, error) {
	v := "0"
	if on {
		v = "1"
	}
	if err := s.settings.Set(ctx, settingKey, v); err != nil {
		return Status{}, err
	}
	if on {
		return s.Check(ctx)
	}
	return s.Status(ctx)
}

// Check asks for the published versions now, when the check is active.
func (s *Service) Check(ctx context.Context) (Status, error) {
	st, err := s.Status(ctx)
	if err != nil || !st.Active() {
		return st, err
	}
	r := s.fetch(ctx)
	s.mu.Lock()
	s.last = &r
	s.mu.Unlock()
	return s.Status(ctx)
}

func (s *Service) fetch(ctx context.Context) result {
	r := result{checkedAt: s.now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		r.err = err
		return r
	}
	resp, err := s.client.Do(req)
	if err != nil {
		r.err = errors.New("could not reach GitHub")
		return r
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		r.err = fmt.Errorf("GitHub answered %d", resp.StatusCode)
		return r
	}
	var list struct {
		Stable struct {
			Version   string    `json:"version"`
			Published time.Time `json:"published"`
		} `json:"stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		r.err = errors.New("the list of versions could not be read")
		return r
	}
	if !IsRelease(list.Stable.Version) {
		r.err = errors.New("no release is published")
		return r
	}
	r.latest, r.published = list.Stable.Version, list.Stable.Published
	return r
}

// Run checks a minute after start, then once a day, while the check is active.
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	run := func() {
		st, err := s.Check(ctx)
		switch {
		case err != nil:
			logger.Error("update check failed", "err", err)
		case st.Err != nil:
			logger.Warn("update check: no answer", "reason", st.Err)
		case st.Available:
			logger.Info("a newer Nutreek is published", "current", st.Current, "latest", st.Latest)
		}
	}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// parseSemver reads "v1.2.3" (a suffix after the patch, like "-rc1", is
// ignored); nil when the version is not one.
func parseSemver(v string) []int {
	v, ok := strings.CutPrefix(v, "v")
	if !ok {
		return nil
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// newer reports whether version a is later than b.
func newer(a, b string) bool {
	x, y := parseSemver(a), parseSemver(b)
	if x == nil || y == nil {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return !prerelease(a) && prerelease(b) // v2.1.0 follows v2.1.0-rc1
}

// prerelease reports whether v has a prerelease suffix, as in "v2.1.0-rc1".
func prerelease(v string) bool {
	v, _, _ = strings.Cut(v, "+")
	return strings.Contains(v, "-")
}
