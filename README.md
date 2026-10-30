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
to `master` to deploy. A normal branch push does not deploy. No release tag is
created by these workflows.

Inspect the server with `systemctl status tgrssbot.service` and
`journalctl -u tgrssbot.service`. Application logs are also under
`/var/lib/tgrssbot/logs`. Configure log retention separately.

## News presentation

News uses a bold headline above the main picture. The first paragraph (up to a
short excerpt for long paragraphs) is visible; remaining feed text is in
Telegram expandable quotes. Each article includes an **Original article** link,
and automatic web previews are disabled. The headline and picture are separate
messages to preserve their order. Text exceeding the photo caption limit follows
in a text message; articles exceeding the message limit continue in collapsed
text messages. Missing or rejected pictures fall back to text.

Content comes from the RSS/Atom entry (full content when available, otherwise
its description). Images use the entry image, Media RSS, image enclosures, or
an inline image. The bot does not scrape the linked article for missing content.
Existing processed-article keys are preserved, so presentation changes do not
resend old news. Delivery is at least once: a failure midway through a multipart
article can cause earlier parts to repeat on the next feed poll.
