# Self-hosting

## Choose the machine

Run the bridge on one persistent machine that stays online. A small Linux VPS,
cloud VM, home server, NAS with Docker support, or Raspberry Pi-class 64-bit
Linux host is sufficient; no GPU is used. Docker Desktop can be used for
evaluation, but sleep and application restarts interrupt message syncing.

The supported deployment model is Linux containers through Docker Compose.
The host must provide:

- outbound internet access for WhatsApp and image downloads;
- a persistent local Docker volume for the linked-device session, database,
  and media;
- a stable HTTPS hostname reachable from the LP3;
- either outbound access for Cloudflare Tunnel or public inbound TCP 80/443 for
  Caddy.

Do not use shared hosting, ephemeral CI runners, or request-based/serverless
services. The bridge maintains a continuous WhatsApp connection and cannot
recreate its state on every request.

## Requirements

- Docker Engine with Docker Compose 2.24.0 or newer.
- Git, Bash, OpenSSL, and curl on the host.
- A persistent machine that stays online.
- Persistent disk sized for the message archive, SQLite state, and downloaded
  media; usage grows with retained media.
- A stable, publicly trusted HTTPS URL reachable by the LP3.
- A primary WhatsApp phone for linked-device pairing.

Clone the repository and confirm Docker before generating secrets:

```bash
git clone https://github.com/kushthakker/lightphone-whatsapp-open-source.git
cd lightphone-whatsapp-open-source
docker version
docker compose version
git --version
openssl version
curl --version
```

Then choose one HTTPS profile below. Do not start both profiles for the same
hostname.

The bridge container runs as UID 10001 and stores all durable state in the
`bridge-data` volume. The origin is bound to `127.0.0.1:8080` on the host and is
also available to proxy containers as `http://bridge:8080`.

## Cloudflare Tunnel profile

This is the NAT-safe route and does not require inbound port forwarding. The
domain must already be active in the same Cloudflare account.

1. Open **Cloudflare Dashboard → Networking → Tunnels**.
2. Select **Create a tunnel**, choose `cloudflared`, give it a name, and finish
   creation. Keep the generated tunnel token private temporarily.
3. Open the tunnel's **Routes** tab, choose **Add route → Published
   application**, and select the hostname used as `PUBLIC_BASE_URL`.
4. Configure the service as:

```text
Type: HTTP
URL:  http://bridge:8080
```

The service name `bridge` resolves inside the Compose network. Do not use
`localhost:8080` here: inside the tunnel container, `localhost` means the
tunnel container itself.

Generate `.env` with the same public hostname configured in the tunnel, then
paste only the saved tunnel token into the empty `TUNNEL_TOKEN=` field. Never
commit the token or paste it into an issue:

```bash
./scripts/init-env.sh https://whatsapp.example.com
```

Start the bridge and tunnel:

```bash
docker compose --profile cloudflare up -d --build
docker compose ps
curl --fail https://whatsapp.example.com/healthz
```

The tunnel should become **Healthy** in Cloudflare. If it does not, inspect
`docker compose logs cloudflared` without posting the token or complete logs.
The tunnel makes outbound-only connections; restrictive networks must permit
the connector's outbound traffic. See Cloudflare's
[current dashboard tunnel guide](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel/).

## Caddy profile

1. Create a public DNS A/AAAA record for the host. On a cloud VM, update its
   security group or firewall; on a home network, configure router forwarding.
2. Forward and allow inbound TCP 80 and 443 to the Docker host. UDP 443 is
   optional for HTTP/3.
3. Run `./scripts/init-env.sh https://whatsapp.example.com`. It sets `DOMAIN`
   and `PUBLIC_BASE_URL` to the same host and creates independent credentials.
4. Start and verify:

```bash
docker compose --profile caddy up -d --build
docker compose ps
curl --fail https://whatsapp.example.com/healthz
```

Replace `whatsapp.example.com` with the hostname passed to `init-env.sh`.

Caddy stores certificates and account state in named volumes. Do not delete
them during routine bridge upgrades. Caddy obtains a publicly trusted
certificate only when DNS resolves to this host and ports 80/443 are reachable;
see Caddy's [automatic HTTPS guide](https://caddyserver.com/docs/quick-starts/https).

For either profile, host firewall rules must never expose TCP 8080. Compose
binds it to `127.0.0.1` only so same-host smoke tests can reach it.

## Verify the private origin

The bridge also binds to loopback for same-host diagnostics. Run:

```bash
./scripts/smoke-compose.sh
```

This checks health, setup-token authorization, and API-token authorization
without printing either credential. It reads token values literally from
`.env`; group names and other configuration are never evaluated as shell code.

Also test the public route from a computer on a different network:

```bash
curl --fail https://whatsapp.example.com/healthz
```

Alternatively, open that `/healthz` URL in a phone browser with Wi-Fi disabled;
it should show the bridge health response rather than a provider login page.

If the local smoke test passes but this public check fails, the bridge is
healthy and the problem is DNS, TLS, firewall/port forwarding, or tunnel
routing—not WhatsApp pairing.

## Pairing and QR expiry

Use the private setup URL printed by `scripts/init-env.sh`. If it is no longer
in the terminal, copy `SETUP_TOKEN` from the ignored `.env` file and open:

```text
https://whatsapp.example.com/setup?token=YOUR_SETUP_TOKEN
```

Replace the hostname and token locally; do not paste this URL into an issue or
chat. If the WhatsApp QR expires before scanning, restart only the bridge
container to request a new QR:

```bash
docker compose restart bridge
```

Once linked, recreating the container with the same `bridge-data` volume must
reconnect without a new QR. Never run `docker compose down -v` unless you
explicitly intend to destroy all bridge state.

After linking and configuring the LP3, set `SETUP_ENABLED=false` in `.env` and
recreate the bridge so Compose reloads the environment:

```bash
docker compose up -d --force-recreate bridge
```

A plain `docker compose restart bridge` does not reload `.env`. Verify the
disabled state with `./scripts/smoke-compose.sh`; it expects the private setup
URL to return 404 when `SETUP_ENABLED=false`. All `/setup*` routes remain
disabled until explicitly re-enabled and recreated again.

## Connect the LP3

The setup page performs two separate pairings:

1. The first QR is a WhatsApp linked-device QR. Scan it from the primary phone
   under **WhatsApp → Settings → Linked devices → Link a device**.
2. After the bridge reports connected, the page displays an app-configuration
   QR containing `PUBLIC_BASE_URL` and `API_TOKEN`.
3. Install the signed APK using [LP3_INSTALL.md](LP3_INSTALL.md), open
   **WhatsApp** on the LP3, and choose **Scan setup QR**.
4. Confirm conversations load, then disable the setup routes as described
   above.

Never send or photograph the setup URL or second QR. Either one grants access
to sensitive bridge configuration.

## Upgrade

Create a volume and `.env` backup using the next section before upgrading. Keep
the named volumes and recreate services from the updated source:

```bash
git pull --ff-only
docker compose --profile cloudflare up -d --build
```

Use `--profile caddy` instead when that is your active route. Do not run
`docker compose down -v`; `-v` deletes the WhatsApp session and archive. After
an upgrade, run `docker compose ps`, `./scripts/smoke-compose.sh`, and the public
`/healthz` check again.

If an upgrade fails, preserve the volume, select the previous source tag with
`git switch --detach PREVIOUS_TAG`, and rerun the same profile command. Return
to current development with `git switch main`. Never attempt recovery by
deleting the volume.

## Backup

For a simple consistent backup, stop the bridge and archive the volume contents,
then start it again. Do not copy only `bridge.db` while the service is running;
WAL mode may leave committed data in `bridge.db-wal`.

The Compose project name is fixed, so the default data volume is
`lp3-whatsapp-bridge_bridge-data`. The volume does **not** contain `.env`;
without `.env`, the LP3 API token, setup token, public URL, and optional tunnel
credential cannot be reproduced unchanged.

Choose an encrypted backup directory outside the repository. A portable
stopped-service backup is:

```bash
backup_dir=/absolute/path/to/encrypted-backup
test -d "$backup_dir"
install -m 600 .env "$backup_dir/bridge.env"
docker compose stop bridge
docker run --rm \
  -v lp3-whatsapp-bridge_bridge-data:/data:ro \
  -v "$backup_dir":/backup \
  alpine:3.22 \
  tar -czf /backup/bridge-data-backup.tgz -C /data .
docker compose start bridge
```

For a host move, use the command above for rehearsal backups only. At the final
cutover, stop the old bridge, create one last archive, and **do not** run
`docker compose start bridge` on the old host. Keep it offline as the rollback
copy. Starting two bridges from the same copied WhatsApp identity can create
competing sessions and divergent archives.

Restore on a new host before starting Compose. The following guard deliberately
refuses to continue if the target volume already exists; extracting over an
existing database or stale SQLite WAL/SHM files can corrupt the restored state.
If the guard fails, stop and back up that existing deployment separately. This
runbook never deletes or overwrites it automatically.

```bash
git clone https://github.com/kushthakker/lightphone-whatsapp-open-source.git
cd lightphone-whatsapp-open-source
install -m 600 /absolute/path/to/encrypted-backup/bridge.env .env
if docker volume inspect lp3-whatsapp-bridge_bridge-data >/dev/null 2>&1; then
  echo "Refusing restore: target data volume already exists" >&2
  exit 1
fi
docker volume create lp3-whatsapp-bridge_bridge-data
docker run --rm \
  -v lp3-whatsapp-bridge_bridge-data:/data \
  alpine:3.22 \
  sh -ec 'test -z "$(find /data -mindepth 1 -maxdepth 1 -print -quit)"'
docker run --rm \
  -v lp3-whatsapp-bridge_bridge-data:/data \
  -v /absolute/path/to/encrypted-backup:/backup:ro \
  alpine:3.22 \
  tar -xzf /backup/bridge-data-backup.tgz -C /data
docker compose --profile cloudflare up -d --build
```

Use the Caddy profile instead when applicable. Restore the same public hostname
and verify `docker compose ps`, `./scripts/smoke-compose.sh`, the public
`/healthz`, and connected status before deciding whether to keep or retire the
offline old host. Never bring the old bridge back online while the restored
bridge is running. If rollback is necessary, stop the new bridge first. If
`.env` cannot be restored, generate new credentials, update the HTTPS route,
and rescan the app-configuration QR. If the public hostname changes, the LP3
must also scan a new configuration QR.

## Troubleshooting

- **No setup QR:** confirm `SETUP_ENABLED=true`, recreate the bridge after
  editing `.env`, and restart only `bridge` if a displayed QR expired.
- **Public `/healthz` fails but the smoke script passes:** fix DNS, the Caddy
  firewall/port forwarding, or the Cloudflare published-application route.
- **502 from Caddy or Cloudflare:** run `docker compose ps` and inspect
  `docker compose logs --tail=100 bridge`; the proxy cannot reach a healthy
  bridge.
- **LP3 reports unauthorized:** the stored API token does not match this
  server. Re-enable setup, recreate `bridge`, scan the new configuration QR,
  and disable setup again.
- **Bridge is healthy but not connected to WhatsApp:** inspect redacted bridge
  logs and confirm the `bridge-data` volume was preserved. Do not delete the
  volume or relink until its backup state is understood.
- **Unexpected JSON in the LP3 app:** verify the configured hostname routes
  directly to this bridge and that `/healthz` and `/api/v1/status` are not being
  replaced by a hosting-provider login page or another application.
