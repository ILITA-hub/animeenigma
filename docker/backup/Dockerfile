FROM alpine:3.21

RUN apk add --no-cache \
    postgresql17-client \
    aws-cli \
    curl \
    bash \
    tzdata

# Set timezone
ENV TZ=Europe/Moscow

COPY backup.sh /usr/local/bin/backup.sh
COPY crontab /etc/crontabs/root

RUN chmod +x /usr/local/bin/backup.sh

# Run crond in foreground
CMD ["crond", "-f", "-l", "2"]
