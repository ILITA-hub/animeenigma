FROM node:20-alpine

WORKDIR /app
COPY server.js ./

ENV PORT=3200

EXPOSE 3200

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -q --spider http://localhost:3200/health || exit 1

CMD ["node", "server.js"]
