# vnstat -> Prometheus exporter.
# Ubuntu 24.04 so the in-container vnstat (2.12) matches the host's exactly —
# the exporter reads a copy of the host SQLite DB, so version parity avoids any
# DB-format skew. stdlib-only Python; no pip deps.
FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends vnstat python3 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY exporter.py /usr/local/bin/exporter.py

# nobody can read the world-readable host DB (ro mount) and write to /tmp.
USER nobody
EXPOSE 9100
ENTRYPOINT ["python3", "/usr/local/bin/exporter.py"]
