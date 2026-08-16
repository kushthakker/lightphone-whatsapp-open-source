# Self-hosting

## Requirements

- Docker Engine with Compose v2.
- A persistent machine that stays online.
- A stable, publicly trusted HTTPS URL reachable by the LP3.
- A primary WhatsApp phone for linked-device pairing.

The bridge container runs as UID 10001 and stores all durable state in the
`bridge-data` volume. The origin is bound to `127.0.0.1:8080` on the host and is
also available to proxy containers as `http://bridge:8080`.

## Cloudflare Tunnel profile

This is the NAT-safe route and does not require inbound port forwarding. The
domain must already be active in the same Cloudflare account.

1. Open **Cloudflare Dashboard → Networking → Tunnels**.
2. Select **Create a tunnel**, choose `cloudflared`, give it a name, and finish
   creation.
3. Copy only the tunnel token from the generated Docker command into the
   ignored `.env` file as `TUNNEL_TOKEN`. Never commit or paste it into an
   issue.
4. Open the tunnel's **Routes** tab, choose **Add route → Published
   application**, and select the hostname used as `PUBLIC_BASE_URL`.
5. Configure the service as:

```text
Type: HTTP
URL:  http://bridge:8080
```

The service name `bridge` resolves inside the Compose network. Do not use
`localhost:8080` here: inside the tunnel container, `localhost` means the
tunnel container itself.

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

1. Create a public DNS A/AAAA record for the host.
2. Forward and allow inbound TCP 80 and 443 to the Docker host. UDP 443 is
   optional for HTTP/3.
3. Set `DOMAIN` to the hostname and `PUBLIC_BASE_URL` to its exact `https://`
   origin in `.env`.
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

## Verify the private origin

The bridge also binds to loopback for same-host diagnostics. Run:

```bash
./scripts/smoke-compose.sh
```

This checks health, setup-token authorization, and API-token authorization
without printing either credential. It reads token values literally from
`.env`; group names and other configuration are never evaluated as shell code.

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

## Backup

For a simple consistent backup, stop the bridge and archive the volume contents,
then start it again. Do not copy only `bridge.db` while the service is running;
WAL mode may leave committed data in `bridge.db-wal`.
