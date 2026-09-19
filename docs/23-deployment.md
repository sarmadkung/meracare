# Deployment

The Go API runs on a DigitalOcean droplet. PostgreSQL and Auth stay on
Supabase, so the droplet holds no data: losing it costs a rebuild, not records.

| | |
|---|---|
| Host | Ubuntu 26.04 LTS, 1 vCPU, 1 GB RAM, `134.209.103.131` |
| Public address | `https://api.rest-api-mock.online` (Caddy terminates TLS) |
| API | `genxcare-api.service`, listening on `127.0.0.1:8090` |
| Database | Supabase session pooler, `DATABASE_MAX_CONNS=5` |
| Deploy | `genxcare-deploy.timer`, every two minutes |

## Releasing

A deploy happens when **`apps/api/VERSION` changes on `main`** — nothing else
does. Commits, merges and mobile releases land without touching the server.

1. Merge the work as usual.
2. Open a pull request that bumps `apps/api/VERSION`, and nothing else.
3. Merge it. Within two minutes the droplet builds that commit and restarts.

Bump `VERSION` last, in its own pull request: a deploy ships everything merged
since the previous one, so the bump is the moment you decide that set is ready.

GitHub Actions is disabled on this account, so nothing tests the code before it
reaches the server. Run `pnpm api:test` and `pnpm api:lint` locally before
merging a bump.

## What a deploy does

`deploy/deploy.sh`, run by the timer:

1. Fetches `origin/main` and compares `apps/api/VERSION` with the deployed
   version in `/opt/genxcare/state/version`. Equal means exit, doing nothing.
2. Builds `cmd/api` and `cmd/migrate` from that commit, one package at a time so
   the build cannot starve the running API of memory.
3. Applies migrations. Each runs in its own transaction under an advisory lock,
   so a deploy that overlaps another waits rather than corrupting anything.
4. Points `/opt/genxcare/bin/api-current` at the new binary and restarts the
   service.
5. Polls `/readyz` for 30 seconds. If the release never becomes ready, the
   previous binary is restored and restarted, and the run fails in the journal.

The version that is live is visible from outside: `/healthz` reports it.

```bash
curl -s https://api.rest-api-mock.online/healthz     # {"status":"ok","version":"0.1.0"}
```

**Rollback restores the binary, not the schema.** Migrations are forward-only,
so a release whose migration is wrong cannot be undone by rolling back — write
migrations that the previous version can still run against.

## The server

Everything the droplet needs is in `deploy/`, installed by `deploy/install.sh`:
swap, firewall, Go, the service user, Caddy, and the three systemd units. It is
safe to re-run and never touches secrets.

```
/opt/genxcare/src      checkout of main, updated by each deploy
/opt/genxcare/bin      built binaries; api-current is the running symlink
/opt/genxcare/state    deployed version and commit
/etc/genxcare/api.env  production configuration and secrets, root:genxcare 0640
```

`/etc/genxcare/api.env` is written by hand and exists only on the server. It
follows `apps/api/.env.example`, with `ENV=production`, `PORT=8090`,
`DATABASE_MAX_CONNS=5` and `PUSH_ENABLED=false`. `SUPABASE_JWT_SECRET` stays
empty: tokens are verified against Supabase's published keys, so the API holds
nothing that could mint one.

## Operating it

```bash
ssh genxcare

systemctl status genxcare-api          # is it running, and since when
journalctl -u genxcare-api -f          # request and scheduler logs
journalctl -u genxcare-deploy -n 50    # what the last deploy did
systemctl start genxcare-deploy        # deploy now instead of waiting
cat /opt/genxcare/state/version        # what is live
```

To roll back by hand, point the symlink at an older build and restart:

```bash
ls /opt/genxcare/bin
ln -sfn /opt/genxcare/bin/api-0.1.0-abc1234 /opt/genxcare/bin/api-current
systemctl restart genxcare-api
```

Then bump `VERSION` to a release that works, or the timer will deploy the
broken one again on its next tick.

## Notes and limits

- **One instance, brief downtime.** A restart drops requests for a second or
  two. Zero-downtime deploys need a second instance and a socket handover,
  which is not worth it at this size.
- **Production shares the Supabase project with development.** Seed personas
  and local testing touch the same data real users would. A separate production
  project is the eventual fix; until then, treat local writes as production
  writes.
- **The database no longer pauses.** The notification scheduler queries every
  minute, which keeps the free-tier project awake.
- **The build runs on the droplet**, peaking near 400 MB against 1 GB of RAM
  plus 1 GB of swap. If the API starts competing with builds for memory, the
  next step is building elsewhere — not a bigger droplet.
- **Mobile builds need the production URL.** Set
  `EXPO_PUBLIC_API_URL=https://api.rest-api-mock.online` for release builds.
