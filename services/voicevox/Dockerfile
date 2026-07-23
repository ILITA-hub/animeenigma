FROM golang:1.25-alpine AS supervisor-builder

WORKDIR /src
COPY services/voicevox/supervisor/go.mod ./
COPY services/voicevox/supervisor/cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /voicevox-supervisor ./cmd/voicevox-supervisor

# Stable official CPU engine with the standard bundled VOICEVOX models,
# including ずんだもん. The host never publishes port 50021.
FROM voicevox/voicevox_engine:cpu-ubuntu24.04-0.25.0

COPY --from=supervisor-builder /voicevox-supervisor /voicevox-supervisor

# The supervisor is PID 1 and starts the official engine as its unprivileged
# user. It removes the heavy child at governor Critical without a Docker socket.
ENTRYPOINT ["/voicevox-supervisor"]
CMD ["gosu", "user", "/opt/voicevox_engine/run", "--host", "0.0.0.0", "--port", "50021", "--cpu_num_threads", "1"]
