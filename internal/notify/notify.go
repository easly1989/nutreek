// Package notify sends the reminders of #237 as Web Push notifications: a minute
// ticker finds what is due for each user in their own time zone, outside their quiet
// hours, once, and sends it to every device they have allowed.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
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
			for _, sub := range subs {
				gone, err := s.Sender.Send(ctx, sub, ev.Message)
				if gone {
					_ = s.Notifications.DeleteSub(ctx, userID, sub.Endpoint)
					continue
				}
				if err != nil {
					if s.Logger != nil {
						s.Logger.Warn("push failed", "user", userID, "err", err)
					}
					continue
				}
				sent++
			}
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
	n := 0
	for _, sub := range subs {
		gone, err := s.Sender.Send(ctx, sub, Message{
			Title: i18n.T(lang, "notify.test.title"), Body: i18n.T(lang, "notify.test.body"), URL: "/settings/notifications", Tag: "nutreek-test",
		})
		if gone {
			_ = s.Notifications.DeleteSub(ctx, userID, sub.Endpoint)
			continue
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
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

// at is the time hhmm on the day of now.
func at(now time.Time, hhmm string) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(time.Duration(minutes(hhmm)) * time.Minute)
}

// due says whether moment fell in the window that ends at now.
func due(now, moment time.Time, window time.Duration) bool {
	return !moment.After(now) && moment.After(now.Add(-window))
}

// Due returns the notifications of reminder r that are due at in.Now.
func Due(r db.Reminder, in Input) []Event {
	if !r.Enabled || !r.OnDay((int(in.Now.Weekday())+6)%7) {
		return nil
	}
	day := in.Now.Format("2006-01-02")
	var out []Event
	switch r.Kind {
	case db.KindWater:
		if in.Water != nil && r.SkipDone && (in.Water.Total >= in.Water.Target || (!in.Water.Last.IsZero() && in.Now.Sub(in.Water.Last) < time.Hour)) {
			return nil
		}
		end := at(in.Now, r.End)
		for slot := at(in.Now, r.Start); !slot.After(end) && r.EveryMin > 0; slot = slot.Add(time.Duration(r.EveryMin) * time.Minute) {
			if due(in.Now, slot, in.Window) {
				body := ""
				if in.Water != nil {
					body = i18n.T(in.Lang, "notify.water.body", "Done", in.Water.Total, "Target", in.Water.Target)
				}
				out = append(out, Event{"water:" + day + ":" + slot.Format("15:04"), Message{
					Title: i18n.T(in.Lang, "notify.water.title"), Body: body, URL: "/", Tag: "nutreek-water"}})
			}
		}
	case db.KindGym:
		if due(in.Now, at(in.Now, r.Start).Add(-time.Duration(r.LeadMin)*time.Minute), in.Window) {
			out = append(out, Event{"gym:" + day, Message{
				Title: i18n.T(in.Lang, "notify.gym.title"), Body: i18n.T(in.Lang, "notify.at", "Time", r.Start), URL: "/", Tag: "nutreek-gym"}})
		}
	case db.KindMeals:
		for _, m := range in.Meals {
			if in.Muted[strings.ToLower(m.Name)] || !due(in.Now, at(in.Now, m.Start).Add(-time.Duration(r.LeadMin)*time.Minute), in.Window) {
				continue
			}
			body := i18n.T(in.Lang, "notify.at", "Time", m.Start)
			if m.Note != "" {
				body = m.Note + " · " + m.Start
			}
			out = append(out, Event{fmt.Sprintf("meal:%d:%s", m.ID, day), Message{Title: m.Name, Body: body, URL: "/", Tag: fmt.Sprintf("nutreek-meal-%d", m.ID)}})
		}
	case db.KindCustom:
		if due(in.Now, at(in.Now, r.Start), in.Window) {
			out = append(out, Event{fmt.Sprintf("custom:%d:%s", r.ID, day), Message{Title: r.Title, URL: "/", Tag: fmt.Sprintf("nutreek-custom-%d", r.ID)}})
		}
	}
	return out
}
