# shelley-share

Read-only sharing of [Shelley](https://exe.dev) conversations over
[tsnet](https://tailscale.com/kb/1244/tsnet/).

The service never binds an HTTP listener on the public internet. It only
serves on a listener obtained from `tsnet.Server`, so only nodes on your
tailnet can reach it. Shareable URLs carry an HMAC over the conversation id;
people on your tailnet can only view conversations you have explicitly given
them a link to.

## How it works

- **Storage:** read-only access to your local `shelley.db`. No writes.
- **Unguessable links:** `/c/<conversation_id>.<base64url(HMAC-SHA256(secret, conversation_id)[:12])>`.
  Without the secret, viewers cannot forge tokens for other conversation ids,
  even if they can guess slugs like `gist-evaluation-and-action`.
- **Secret:** derived (via HMAC under a domain-separation label) from an
  existing machine-local file — by default `~/.ssh/id_ed25519`, which is
  present on every exe.dev VM. No new long-lived secret needs to be created or
  backed up. If no SSH key exists, falls back to a random secret under the
  state directory.
- **Best-effort secret redaction:** before rendering, message text and tool
  input/output are run through [Trufflehog](https://github.com/trufflesecurity/trufflehog)'s
  default detectors (~800 vendor-specific rules, e.g. AWS, GitHub, Stripe,
  Slack, OpenAI). Detected secret literals are replaced with `〈redacted〉`.
  Verification is disabled, so we never call out to third-party APIs. This is
  best-effort, not a complete DLP — disable with `-no-redact` if you really
  trust your audience.
- **Transport safety:** the HTTP server is constructed inside
  `internal/server` and is fed *only* a listener returned by
  `tsnet.Server.Listen`. The package guards against accidentally being handed
  a stdlib `*net.TCPListener` at runtime, and the handler is unexported so
  callers cannot pass it to `http.ListenAndServe` from outside.

## Setup

```sh
go install github.com/lukasschwab/shelley-share@latest

# First run: provide a Tailscale auth key. The node registers as
# "shelley-share" by default.
TS_AUTHKEY=tskey-auth-... shelley-share serve
```

State (tsnet keys, the base URL hint, optional random secret) lives in
`~/.config/shelley-share/`. The Shelley database is read from
`~/.config/shelley/shelley.db` by default; override with `-db` or
`$SHELLEY_DB`.

A reasonable systemd unit (see exe.dev's `<systemd>` guidance):

```ini
[Unit]
Description=shelley-share (tsnet read-only viewer)
After=network-online.target
Wants=network-online.target

[Service]
User=exedev
Environment=TS_AUTHKEY=tskey-auth-...
ExecStart=/home/exedev/go/bin/shelley-share serve
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Sharing a conversation

On the Shelley machine:

```sh
shelley-share link c2SEXJF
# → http://shelley-share/c/c2SEXJF.AbCdEfGhIjKlMnOp
```

Send that URL to a coworker on your tailnet. They can read the conversation;
they cannot construct URLs for other conversations. Revoke all outstanding
links by rotating the seed file (e.g. regenerating `~/.ssh/id_ed25519`) or
pointing `-seed` at a different file.

## Usage

```
shelley-share help                       # top-level help
shelley-share serve -h                   # serve flags
shelley-share link -h                    # link flags
shelley-share link <conversation_id>     # print a shareable URL
```

Find conversation ids with the Shelley CLI (`shelley client list`) or by
querying `shelley.db` directly.

## Caveats

- Anyone on your tailnet with a valid token can view that conversation. This
  is share-by-link, not per-user ACLs. (Per-identity gating via
  `tailscale.LocalClient.WhoIs` would be a natural extension.)
- Tool input/output is presented in collapsed `<details>` blocks. There is no
  JS; expand-by-default is a browser feature, not an auth boundary. Treat
  links as you would a Google Doc "anyone with the link" share.
