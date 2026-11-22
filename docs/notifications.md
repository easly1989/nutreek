# Notifications

Reminders (water, training, meals from the meal times, custom ones) reach the installed PWA as Web Push notifications, even when the app is closed. The page is `/settings/notifications`.

## How it works

- **Keys.** The first start makes a VAPID key pair and keeps it in `instance_settings` (`vapid_public`, `vapid_private`). There is no environment variable and no outside account: the notifications go straight to each browser's own push service. Deleting the two rows makes new keys, and every device must then turn the notifications on again.
- **Devices.** A device is a push subscription (`push_subscriptions`). The page's script asks the permission, subscribes with the public key and sends the subscription and the browser's time zone. A push service that answers 404 or 410 for a device gets it removed.
- **Sending.** `internal/notify` ticks every minute. For each user with a device and a reminder that is on, it works in the user's time zone, skips the quiet hours, and sends what fell in the last two minutes. `notification_log` keeps what was sent for two days so a reminder goes out once.
- **Reminders.** Water (a slot every N minutes between two times, skipped when the goal is reached or water was logged in the last hour), training (a time, minutes before, days), meals (the times of the meals and reminders of the day, minutes before, per meal name), custom (a title, a time, days). One quiet-hours band per user applies to all.
- **Demo build.** No sender runs and the page says so.

## What it needs

- **HTTPS.** Browsers allow push only on a secure origin (`localhost` also works for trying it). Behind a reverse proxy, serve the site over HTTPS.
- **iPhone and iPad.** Safari sends push only to an app added to the Home Screen (iOS 16.4 or later). The page tells a user who is in a browser tab to install the app first.
- **A browser with the Push API.** Without it the page says the notifications are not available.

## What is not checked

The sending was tested against a local server that answers like a push service, and the page against a real server. Delivery through Apple's, Google's and Mozilla's push services, and a reverse proxy in front, need a try on a real installation.
