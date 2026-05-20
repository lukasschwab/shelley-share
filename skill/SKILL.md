---
name: shelley-share
description: Use when the user asks to share a Shelley conversation with a coworker, generate a shelley-share link, or debug the shelley-share systemd service.
---

`shelley-share` is a tsnet-only read-only viewer for Shelley conversations
running on the user's exe.dev VM. Repo:
https://github.com/lukasschwab/shelley-share.

It listens **only on the user's tailnet** (never the public internet) and
renders conversations at `/c/<conv_id>.<hmac-tag>`. The tag is HMAC-SHA256 of
the conversation id under a key derived from `~/.ssh/id_ed25519`; coworkers
on the tailnet can only open links the user has explicitly minted.

## Publishing the current conversation

The current conversation id is in `$SHELLEY_CONVERSATION_ID`:

```bash
shelley-share link "$SHELLEY_CONVERSATION_ID"
```

Print the URL and tell the user they can send it to anyone on their tailnet.
For a specific past conversation, pass the id explicitly:

```bash
shelley-share link c2SEXJF
```

If `link` errors with "conversation … not found", the id is wrong or in a
different database; check with:

```bash
sqlite3 ~/.config/shelley/shelley.db \
  "SELECT conversation_id, slug FROM conversations
   WHERE conversation_id LIKE '%<fragment>%' OR slug LIKE '%<fragment>%';"
```

## Debugging the systemd service

The service unit is `shelley-share.service`. Standard checks:

```bash
systemctl status shelley-share          # is it active?
journalctl -u shelley-share -n 100      # recent logs
journalctl -u shelley-share -f          # follow live
sudo systemctl restart shelley-share    # restart after binary update
```

Expected startup lines:

```
shelley-share: secret derived from /home/exedev/.ssh/id_ed25519
shelley-share: redaction enabled (trufflehog default detectors)
shelley-share: tailnet name <name>.<tailnet>.ts.net
shelley-share: listening on tsnet :80
```

### Common problems

- **"tsnet up: … not authenticated"** on first start: needs an auth key.
  Stop the service, run once interactively with
  `TS_AUTHKEY=tskey-auth-... /home/exedev/go/bin/shelley-share serve` to
  register, then `sudo systemctl start shelley-share`. Keys persist in
  `~/.config/shelley-share/tsnet/`.
- **Link 404s for the coworker:** they're not connected to the right
  tailnet, or the token was truncated in transit (no trailing characters
  after the final `.`).
- **Link 404s for the operator too:** the conversation id doesn't exist in
  `~/.config/shelley/shelley.db`. Confirm with the SQL query above.
- **"refusing to serve on a non-tsnet listener"**: someone refactored the
  code; this is a safety check and means the binary should not be deployed.
- **Binary outdated**: rebuild and restart:

  ```bash
  cd ~/src/shelley-share && go install . && sudo systemctl restart shelley-share
  ```

### Toggling redaction

Secrets are redacted by Trufflehog's default detectors before any HTML is
served. To disable (NOT recommended), edit
`/etc/systemd/system/shelley-share.service`, append `-no-redact` to
`ExecStart`, then `sudo systemctl daemon-reload && sudo systemctl restart
shelley-share`.

### Rotating the share secret

All outstanding links can be invalidated by changing the seed file (e.g.
regenerating `~/.ssh/id_ed25519`) or pointing `-seed` at a different file.
After rotation, restart the service and re-mint any links the user still
wants to share.
