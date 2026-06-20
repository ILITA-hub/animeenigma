# stealth-scraper — Camoufox (anti-detect Firefox) browser-scraping sidecar.
# Internal-only service on the Docker network; the Go scraper is the only caller.
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    HOST=0.0.0.0 \
    PORT=3000 \
    STEALTH_PROFILE_DIR=/data/profiles

WORKDIR /app

COPY requirements.txt .
# Install deps, the system libraries headless Firefox needs (via Playwright's
# dep installer), then fetch the Camoufox Firefox binary into the image.
RUN pip install -r requirements.txt \
 && python -m playwright install-deps firefox \
 && python -m camoufox fetch \
 && mkdir -p /data/profiles

COPY app ./app

EXPOSE 3000

# /healthz needs no browser, so it is up well before the pool warms.
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 \
  CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:3000/healthz').status==200 else 1)"

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "3000"]
