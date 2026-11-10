# Running your own Parallax server

Parallax is one Go binary with the web client built in. It keeps games in memory, saves open rooms to a folder so a
restart doesn't end them, and needs no database. Any small Linux machine works: a Raspberry Pi, a cloud VM, a home
server.

## 1. Build

You need Go and Node.js on the machine you build on (it doesn't have to be the server).

```sh
make linux-arm64   # bin/parallax-linux-arm64: Raspberry Pi, AWS Graviton, Ampere
make linux-amd64   # bin/parallax-linux-amd64: most other servers
```

Try it locally first with `make build && bin/server`, then open http://localhost:8080.

## 2. Install

Copy the binary to the server, for example as `/usr/local/bin/parallax`. Then install a systemd unit; start from
[deploy/examples/parallax.service](deploy/examples/parallax.service):

```sh
sudo install -m 755 parallax-linux-arm64 /usr/local/bin/parallax
sudo cp parallax.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now parallax
curl http://127.0.0.1:8080/healthz   # ok
```

The server flags that matter:

| Flag | What it does |
| --- | --- |
| `-addr 127.0.0.1:8080` | Where to listen. Keep it on loopback behind a proxy. |
| `-state /var/lib/parallax/rooms` | Save open rooms there every 30 s and on shutdown, and reopen them at start. Without it, a restart ends every game. |
| `-public-url https://game.example.com` | The address players use, for link previews. By default it's worked out from each request. |
| `-origin game.example.com` | Extra hosts whose pages may connect, if your proxy rewrites the `Host` header. |
| `-system1 URL` | Optional: the System 1 model's `/predict` endpoint for model-driven computer players. See below. |

`bin/server -h` lists the rest.

## 3. Put it behind HTTPS

Players connect over a WebSocket at `/ws`, so the proxy must pass WebSocket upgrades.

- **Caddy** gets a certificate and handles WebSockets on its own: [deploy/examples/Caddyfile](deploy/examples/Caddyfile).
- **nginx** needs the upgrade headers: [deploy/examples/nginx.conf](deploy/examples/nginx.conf).
- **Apache** needs `mod_proxy_wstunnel` (or `ProxyPass ... upgrade=websocket`) and `ProxyPreserveHost On`.
- **Tailscale:** `tailscale serve --bg --https=443 http://127.0.0.1:8080` serves it to your tailnet with no other setup.

## 4. Update it later

[`scripts/deploy.sh`](scripts/deploy.sh) builds, ships, restarts and checks a server over ssh. Describe your servers
in a `.env` file at the repo root. Copy [.env.example](.env.example) and fill it in. It's gitignored, so your hosts
stay out of the repo.

```sh
cp .env.example .env              # then edit: ssh alias, URL, port, layout
scripts/deploy.sh mygame          # build main, ship it, restart, check
scripts/deploy.sh mygame --check  # check what's live
scripts/deploy.sh mygame --rollback
```

The script replaces only the binary: set up the unit and the proxy yourself first, as above. It refuses to restart a
unit without `-state`, so a deploy never ends a game in progress. Two install layouts are supported:

- **single**: one binary, with the previous one kept for rollback.
- **releases**: one folder per version, with a `current` link.

## Optional: System 1 computer players

The rule-based computer players need nothing extra. The System 1 players ask a small language model. It needs about
1.5 GB of memory and runs fine on a CPU. [deploy/laya/README.md](deploy/laya/README.md) sets it up. Then add
`-system1 http://127.0.0.1:8799/predict` to the server's flags, with the shared key in `PARALLAX_SYSTEM1_KEY`. If the
model is unreachable, those players play by the rules instead.

Load limits, so one server can't swamp the model:

- `-system1-concurrency 2`: the most model requests in flight at once, across every game.
- `-system1-wait 2s`: how long a turn waits for a free slot before playing Planner.
- `-system1-games 10`: the most games at once with System 1 players. Past it, lobbies stop offering them and games
  with them can't start until one finishes.

Every five minutes, while the numbers change, the server logs a "System 1 use" line: decisions asked and answered,
fallbacks by reason, requests in flight and games using it.

## Link previews and images

Shared links show [web/public/og.jpg](web/public/og.jpg). The page's preview tags point at your own server
automatically. `make shots` regenerates the image and the README pictures from a seeded game.
