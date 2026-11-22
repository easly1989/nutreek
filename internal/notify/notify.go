// Package notify sends the reminders of #237 as Web Push notifications: a minute
// ticker finds what is due for each user in their own time zone, outside their quiet
// hours, once, and sends it to every device they have allowed.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/i18n"
)

// Message is what a notification shows; Tag makes a newer one replace an older one.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	Icon  string `json:"icon,omitempty"` // set when sending: the app icon
}

// icon is the picture a notification carries.
const icon = "/static/icons/icon-bosco-192.png"

// Sender sends a message to one device. gone is true when the push service says the
// device no longer exists (the caller forgets it).
type Sender interface {
	Send(ctx context.Context, sub db.PushSub, msg Message) (gone bool, err error)
}

// VAPID is the key pair that signs the pushes, made on the first start and kept in
// instance_settings (no environment variable, no outside service).
type VAPID struct{ Public, Private string }

// EnsureVAPID returns the instance's VAPID keys, making them when there are none.
func EnsureVAPID(ctx context.Context, settings *db.InstanceSettings) (VAPID, error) {
	pub, okPub, err := settings.Get(ctx, "vapid_public")
	if err != nil {
		return VAPID{}, err
	}
	priv, okPriv, err := settings.Get(ctx, "vapid_private")
	if err != nil {
		return VAPID{}, err
	}
	if okPub && okPriv && pub != "" && priv != "" {
		return VAPID{Public: pub, Private: priv}, nil
	}
	priv, pub, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return VAPID{}, fmt.Errorf("generate VAPID keys: %w", err)
	}
	if err := settings.Set(ctx, "vapid_private", priv); err != nil {
		return VAPID{}, err
	}
	if err := settings.Set(ctx, "vapid_public", pub); err != nil {
		return VAPID{}, err
	}
	return VAPID{Public: pub, Private: priv}, nil
}

// WebPush sends through the browsers' own push services, signed with the VAPID keys.
type WebPush struct {
	Keys       VAPID
	Subscriber string // a mailto: or https: contact the push service can use
	Client     *http.Client
}

// Send implements Sender.
func (w WebPush) Send(ctx context.Context, sub db.PushSub, msg Message) (bool, error) {
	msg.Icon = icon
	payload, err := json.Marshal(msg)
	if err != nil {
		return false, err
	}
	opts := &webpush.Options{
		Subscriber:      w.Subscriber,
		VAPIDPublicKey:  w.Keys.Public,
		VAPIDPrivateKey: w.Keys.Private,
		TTL:             3600,
		Urgency:         webpush.UrgencyNormal,
	}
	if w.Client != nil { // not a nil *http.Client in the interface: the library would call it
		opts.HTTPClient = w.Client
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, opts)
	if err != nil {
		// The endpoint is a secret URL: keep it out of the error that gets logged.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = fmt.Errorf("push request failed: %w", ue.Err)
		}
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	// 404 and 410: the device is gone. 401 and 403: the push service no longer accepts
	// our key for it (new keys, or a subscription made with another one); it needs a new
	// subscription either way.
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone ||
		resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return true, nil
	case resp.StatusCode >= 400:
		return false, fmt.Errorf("push service answered %d", resp.StatusCode)
	}
	return false, nil
}

// Service finds what is due and sends it.
type Service struct {
	Notifications *db.Notifications
	Users         *db.Users
	Plans         *db.WeeklyPlans
	Water         *db.WaterLogs
	Sender        Sender
	Logger        *slog.Logger
	// Window is how far back a moment still counts as due when a tick was late.
	Window time.Duration
}

// Run ticks every minute until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if _, err := s.Tick(ctx, now); err != nil && s.Logger != nil {
				s.Logger.Warn("notifications tick failed", "err", err)
			}
			if i%60 == 0 {
				_ = s.Notifications.ForgetOldSent(ctx)
			}
		}
	}
}

// Tick sends what is due at now and returns how many notifications it sent.
func (s *Service) Tick(ctx context.Context, now time.Time) (int, error) {
	all, err := s.Notifications.Enabled(ctx)
	if err != nil {
		return 0, err
	}
	byUser := map[int64][]db.Reminder{}
	for _, r := range all {
		byUser[r.UserID] = append(byUser[r.UserID], r)
	}
	window := s.Window
	if window <= 0 {
		window = 2 * time.Minute
	}
	sent := 0
	for userID, reminders := range byUser {
		n, err := s.tickUser(ctx, userID, reminders, now, window)
		if err != nil && s.Logger != nil {
			s.Logger.Warn("notifications for a user failed", "user", userID, "err", err)
		}
		sent += n
	}
	return sent, nil
}

func (s *Service) tickUser(ctx context.Context, userID int64, reminders []db.Reminder, now time.Time, window time.Duration) (int, error) {
	cfg, err := s.Notifications.Settings(ctx, userID)
	if err != nil {
		return 0, err
	}
	loc := time.UTC
	if l, err := time.LoadLocation(cfg.TimeZone); err == nil && cfg.TimeZone != "" {
		loc = l
	}
	local := now.In(loc)
	if cfg.QuietOn && InQuiet(local, cfg.QuietFrom, cfg.QuietTo) {
		return 0, nil
	}
	user, err := s.Users.GetByID(ctx, userID)
	if err != nil {
		return 0, err
	}
	lang := user.Locale
	if lang == "" {
		lang = i18n.Default
	}
	subs, err := s.Notifications.Subs(ctx, userID)
	if err != nil || len(subs) == 0 {
		return 0, err
	}
	in := Input{Now: local, Window: window, Lang: lang}
	for _, r := range reminders {
		switch r.Kind {
		case db.KindMeals:
			if in.Meals == nil {
				if in.Meals, err = s.Plans.MealSlotsOn(ctx, userID, local); err != nil {
					return 0, err
				}
				if in.Muted, err = s.Notifications.MealMutes(ctx, userID); err != nil {
					return 0, err
				}
			}
		case db.KindWater:
			if in.Water == nil {
				total, last, err := s.Water.StateOn(ctx, userID, local)
				if err != nil {
					return 0, err
				}
				target := 2000
				if user.WaterTargetML > 0 {
					target = user.WaterTargetML
				}
				w := WaterState{Total: total, Target: target, Last: last}
				in.Water = &w
			}
		}
	}
	sent := 0
	for _, r := range reminders {
		for _, ev := range Due(r, in) {
			first, err := s.Notifications.MarkSent(ctx, userID, ev.Key)
			if err != nil {
				return sent, err
			}
			if !first {
				continue
			}
			n, err := s.push(ctx, userID, subs, ev.Message)
			if err != nil && s.Logger != nil {
				s.Logger.Warn("push failed", "user", userID, "err", err)
			}
			sent += n
		}
	}
	return sent, nil
}

// SendTest sends a notification to every device of userID, to try them.
func (s *Service) SendTest(ctx context.Context, userID int64, lang string) (int, error) {
	subs, err := s.Notifications.Subs(ctx, userID)
	if err != nil {
		return 0, err
	}
	return s.push(ctx, userID, subs, Message{
		Title: i18n.T(lang, "notify.test.title"), Body: i18n.T(lang, "notify.test.body"), URL: "/settings/notifications", Tag: "nutreek-test",
	})
}

// push sends msg to every device of userID at once (a slow one does not hold up the
// others), forgets the ones the push service says are gone, and returns how many
// were reached and the first error of the others.
func (s *Service) push(ctx context.Context, userID int64, subs []db.PushSub, msg Message) (int, error) {
	type result struct {
		gone bool
		err  error
	}
	results := make([]result, len(subs))
	var wg sync.WaitGroup
	for i, sub := range subs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i].gone, results[i].err = s.Sender.Send(ctx, sub, msg)
		}()
	}
	wg.Wait()
	n := 0
	var first error
	for i, r := range results {
		switch {
		case r.gone:
			_ = s.Notifications.DeleteSub(ctx, userID, subs[i].Endpoint)
		case r.err != nil:
			if first == nil {
				first = r.err
			}
		default:
			n++
		}
	}
	return n, first
}

// ---- what is due (pure) ------------------------------------------------------

// WaterState is what the water reminder looks at.
type WaterState struct {
	Total  int       // ml today
	Target int       // ml
	Last   time.Time // when water was last logged today (zero when never)
}

// Input is what Due needs besides the reminder: the user's local time and day.
type Input struct {
	Now    time.Time // in the user's time zone
	Window time.Duration
	Lang   string
	Meals  []db.MealSlot   // today's meals with a time, for the «Meals» reminder
	Muted  map[string]bool // meal names (lower case) it stays quiet for
	Water  *WaterState
}

// Event is one notification that is due, with the key that makes it go out once.
type Event struct {
	Key     string
	Message Message
}

// InQuiet says whether the local time falls in the quiet band from..to («22:00» to
// «07:00» runs through midnight).
func InQuiet(local time.Time, from, to string) bool {
	m := local.Hour()*60 + local.Minute()
	f, t := minutes(from), minutes(to)
	if f == t {
		return false
	}
	if f < t {
		return m >= f && m < t
	}
	return m >= f || m < t
}

func minutes(hhmm string) int {
	var h, m int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return 0
	}
	return h*60 + m
}

// atMin is the wall-clock time mins minutes after midnight on the day of day (a
// negative or large value runs into the day before or after). It is built from the
// calendar, not by adding a duration, so 08:00 stays 08:00 on the days the clocks change.
func atMin(day time.Time, mins int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, mins, 0, 0, day.Location())
}

// at is the time hhmm on the day of now.
func at(now time.Time, hhmm string) time.Time { return atMin(now, minutes(hhmm)) }

// due says whether moment fell in the window that ends at now.
func due(now, moment time.Time, window time.Duration) bool {
	return !moment.After(now) && moment.After(now.Add(-window))
}

// Due returns the notifications of reminder r that are due at in.Now.
func Due(r db.Reminder, in Input) []Event {
	if !r.Enabled {
		return nil
	}
	day := in.Now.Format("2006-01-02")
	var out []Event
	onToday := r.OnDay((int(in.Now.Weekday()) + 6) % 7)
	switch r.Kind {
	case db.KindWater:
		if !onToday || (in.Water != nil && r.SkipDone && (in.Water.Total >= in.Water.Target || (!in.Water.Last.IsZero() && in.Now.Sub(in.Water.Last) < time.Hour))) {
			return nil
		}
		start, end := minutes(r.Start), minutes(r.End)
		for slot := start; slot <= end && r.EveryMin > 0; slot += r.EveryMin {
			if due(in.Now, atMin(in.Now, slot), in.Window) {
				body := ""
				if in.Water != nil {
					body = i18n.T(in.Lang, "notify.water.body", "Done", in.Water.Total, "Target", in.Water.Target)
				}
				out = append(out, Event{"water:" + day + ":" + atMin(in.Now, slot).Format("15:04"), Message{
					Title: i18n.T(in.Lang, "notify.water.title"), Body: body, URL: "/", Tag: "nutreek-water"}})
			}
		}
	case db.KindGym:
		// Today's session and tomorrow's: with a lead time, a session just after
		// midnight is announced late the evening before.
		for off := 0; off <= 1; off++ {
			session := in.Now.AddDate(0, 0, off)
			if !r.OnDay((int(session.Weekday())+6)%7) || !due(in.Now, atMin(session, minutes(r.Start)-r.LeadMin), in.Window) {
				continue
			}
			out = append(out, Event{"gym:" + session.Format("2006-01-02"), Message{
				Title: i18n.T(in.Lang, "notify.gym.title"), Body: i18n.T(in.Lang, "notify.at", "Time", r.Start), URL: "/", Tag: "nutreek-gym"}})
		}
	case db.KindMeals:
		if !onToday {
			return nil
		}
		for _, m := range in.Meals {
			if in.Muted[strings.ToLower(m.Name)] || !due(in.Now, atMin(in.Now, minutes(m.Start)-r.LeadMin), in.Window) {
				continue
			}
			body := i18n.T(in.Lang, "notify.at", "Time", m.Start)
			if m.Note != "" {
				body = m.Note + " · " + m.Start
			}
			out = append(out, Event{fmt.Sprintf("meal:%d:%s", m.ID, day), Message{Title: m.Name, Body: body, URL: "/", Tag: fmt.Sprintf("nutreek-meal-%d", m.ID)}})
		}
	case db.KindCustom:
		if onToday && due(in.Now, at(in.Now, r.Start), in.Window) {
			out = append(out, Event{fmt.Sprintf("custom:%d:%s", r.ID, day), Message{Title: r.Title, URL: "/", Tag: fmt.Sprintf("nutreek-custom-%d", r.ID)}})
		}
	}
	return out
}
