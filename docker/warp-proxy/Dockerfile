# Cloudflare WARP egress in PROXY mode — a non-datacenter exit for the
# solve_challenge scraper providers (miruro/animepahe) whose Cloudflare managed
# __cf_chl_rt_tk challenge is unpassable from our server's datacenter IP
# (proven 2026-07-20). WARP proxy mode is userspace (SOCKS5 on 127.0.0.1:40000,
# no TUN/NET_ADMIN); socat republishes it on 0.0.0.0:1080 so sibling containers
# reach it as socks5://warp-proxy:1080.
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl gnupg socat lsb-release \
    && curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg \
        | gpg --yes --dearmor -o /usr/share/keyrings/cloudflare-warp-archive-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg] https://pkg.cloudflareclient.com/ $(lsb_release -cs) main" \
        > /etc/apt/sources.list.d/cloudflare-client.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends cloudflare-warp \
    && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# SOCKS5 exposed to the docker network.
EXPOSE 1080

ENTRYPOINT ["/entrypoint.sh"]
