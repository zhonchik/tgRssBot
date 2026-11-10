# tgRssBot

Telegram bot for aggregate RSS feeds

## Build and validation

Use Go 1.26.x. Before committing a dependency update, run:

```sh
go mod download all
go mod verify
go test -mod=readonly ./...
go vet -mod=readonly ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath -o dist/tgRssBot .
```

Commit the generated `go.sum`. CI uses the same checks and archives a Linux amd64 binary.

## Production systemd setup

Target: `v245006.hosted-by-vdsina.com`, SSH user `tgrssbot-deploy`.
The service runs as a separate unprivileged `tgrssbot` user. The application reads
`config.yaml`, maintains `storage.db`, and writes `logs/` in `/var/lib/tgrssbot`.
Deployments replace only the executable. Back up the database before migrating an
existing installation; stop the old process before copying its database here.

As an administrator on the server, from a checkout of this repository:

```sh
sudo useradd --system --home-dir /var/lib/tgrssbot --shell /usr/sbin/nologin tgrssbot
# Create tgrssbot-deploy with an SSH login if it does not already exist.
sudo install -d -o root -g root -m 755 /opt/tgrssbot
sudo install -d -o tgrssbot-deploy -g tgrssbot-deploy -m 700 /opt/tgrssbot/incoming
sudo install -d -o tgrssbot -g tgrssbot -m 700 /var/lib/tgrssbot /var/lib/tgrssbot/logs
sudo install -o tgrssbot -g tgrssbot -m 600 deploy/config.example.yaml /var/lib/tgrssbot/config.yaml
sudoedit /var/lib/tgrssbot/config.yaml
sudo install -o root -g root -m 755 deploy/tgrssbot-activate /usr/local/sbin/tgrssbot-activate
sudo install -o root -g root -m 644 deploy/tgrssbot.service /etc/systemd/system/tgrssbot.service
sudo visudo -cf deploy/tgrssbot.sudoers
sudo install -o root -g root -m 440 deploy/tgrssbot.sudoers /etc/sudoers.d/tgrssbot
sudo systemctl daemon-reload
sudo systemctl enable tgrssbot.service
```

Set the real Telegram token in `config.yaml` before the first deployment. Do not
commit it. Install the deployment public key in `tgrssbot-deploy`'s
`~/.ssh/authorized_keys`, ideally with the `restrict` option. The deployment user
needs write access only to the incoming directory and sudo access only to the
root-owned activation helper. The service stops with SIGINT for graceful shutdown.
Ensure the server has CA certificates, systemd, Bash, `flock`, sudo, and an SSH/SFTP
server, plus outbound HTTPS access to Telegram and the configured feeds.

## Automated deployment

The GitHub `production` environment must contain:

| Secret | Value |
| --- | --- |
| `DEPLOY_SSH_KEY` | SSH private key for `tgrssbot-deploy` |
| `DEPLOY_KNOWN_HOSTS` | Pinned OpenSSH known_hosts entry for `v245006.hosted-by-vdsina.com` |

Verify the host key fingerprint through the provider console or another trusted
channel before saving it. Strict host key checking is required by the workflow.

Only a pushed stable tag such as `v1.2.3` can deploy. Prereleases, build metadata,
and tags whose commits are outside `master` fail validation. The workflow tests and
builds the tagged commit before entering the protected `production` environment.
Deployments are serialized. The server keeps the preceding executable and restores
it when the new service fails its ten-second systemd activity check. This is a
process check; it does not verify Telegram connectivity or bot responses.

Once setup and CI are verified, create and push a stable tag on a commit belonging
to `master` to deploy. The Build workflow runs on pushes to `master`. The Deploy
workflow runs only on pushed version tags (`v*`) and validates, builds, and deploys
the tagged commit. No release tag is created by these workflows.

Inspect the server with `systemctl status tgrssbot.service` and
`journalctl -u tgrssbot.service`. Application logs are also under
`/var/lib/tgrssbot/logs`. Configure log retention separately.

## News presentation

News sends the main picture separately, followed by a text message with a bold headline.
The first paragraph (up to a short excerpt for long paragraphs) is visible;
remaining article text is in Telegram expandable quotes. Each article includes an
**Source** link, and automatic web previews are disabled. Article text uses the 4,096-character message limit instead of the smaller photo
caption limit. Text is shortened with an ellipsis to fit the message limit,
with the Source link retained for reading the full article. Missing or rejected
pictures fall back to a single text message.

For new articles, the bot fetches the linked HTML page and extracts its main
body with Readability. Requests have a 15-second timeout and a 5 MiB size limit.
Blocked, failed, or empty pages fall back to RSS/Atom content or its description.
JavaScript-only and paywalled text may be unavailable. Images use feed metadata
or an inline image, with the page image as a fallback. Extraction is generic;
individual publishers do not require configured selectors.
Existing processed-article keys are preserved, so presentation changes do not
resend old news. Delivery is at least once: an ambiguous transport failure can cause a message
to repeat on the next feed poll.

## Subscription settings in Telegram

Set `bot.settings_url` in config.yaml to the public HTTPS root of the settings
site. Forward that site's requests to `127.0.0.1:8080` using your HTTPS reverse
proxy. Override the local address with `bot.settings_listen` if necessary.
Restart the bot and send /settings in its private chat. The app validates
Telegram's signed launch data and scopes every operation to the launching user.
Launches expire after one hour; reopen the app to continue.

Existing subscriptions default to unpaused, every minute, all items, and no
keyword filter. Delivery checks and filters are per subscription. Feed downloads
remain shared. The item limit selects the latest matching entries before
deduplication.


### In-chat settings

Send `/settings` in a private chat with the bot to manage subscriptions without
configuring a web server. Select a feed to pause or resume delivery, change its
interval (1–1440 minutes), set its item limit (0–100, with 0 meaning all), or edit
or clear its keyword filter (up to 120 characters). Numeric values and keywords
are entered in reply to the menu prompt; `/cancel` returns to the menu.
You can also add feeds and remove subscriptions with confirmation. Changes use
the same persisted settings as the web interface. When `settings_url` is set,
the menu also includes a link to that interface.

### Calendar pauses

Add a public iCalendar (`.ics`) subscription URL. For example, the F1 Calendar
project publishes https://f1.vidmar.net/calendar.ics (verified to contain the
2026 schedule). This full feed includes practice, qualifying, races and testing;
choose a race-only feed if you only want race boundaries.

```
/calendar add https://f1.vidmar.net/calendar.ics
/calendar list
/calendar link https://example.com/rss https://f1.vidmar.net/calendar.ics
/calendar next https://example.com/rss
/resume https://example.com/rss
/calendar advance https://f1.vidmar.net/calendar.ics
/calendar resume https://f1.vidmar.net/calendar.ics
/calendar unlink https://example.com/rss
/calendar delete https://f1.vidmar.net/calendar.ics
```

Calendars belong to the requesting user and can control many subscriptions.
Linking arms the subscription for the next future event. Items received at or
after that event's start are queued until you advance or resume. `next` advances
one subscription by one event; `advance` does so for all subscriptions linked to
the calendar. This releases items received before the following event starts,
allowing you to catch up one session at a time. `resume` releases the entire
backlog and arms the next future event. Manual `/pause` still takes precedence.
At the last event, advancing releases all remaining items. With no future event,
delivery continues until an hourly refresh supplies one.

Calendars and progress survive restart. Feeds refresh at startup and every hour;
failed downloads retain the last valid schedule. Deleting a calendar unlinks its
subscriptions without deleting queued articles. UTC, timezone-qualified and
all-day events are supported (floating times use the server timezone). Recurrence
rules are rejected: use feeds listing each event explicitly, like the F1 feed.
Article boundaries use the time the bot received each article, not its publication
time. Legacy queue items without timestamps stay held at a boundary until resume.

When a calendar event triggers a pause, the bot sends a **Subscription paused**
notification naming the feed, event, calendar URL and event start time in UTC.
The check runs at least once a minute, even when there are no new articles.
Successful notifications are recorded across restarts; failed sends are retried
(ambiguous delivery failures can produce duplicates). `/list`, the in-chat
settings menu and web settings show the pause reason, including manual pauses.

You can also manage calendars entirely through `/settings`: choose **Calendars**
to add, inspect or delete a calendar (with confirmation), or resume/advance all
linked subscriptions. Open a subscription to **Link / change calendar**,
**Unlink calendar**, or **Advance to next event**. The existing **Resume delivery**
button resumes that subscription. Adding a calendar prompts for its subscription
URL; `/cancel` returns to the main settings menu.
