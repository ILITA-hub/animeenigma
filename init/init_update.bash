
# we in the root of the project

ROOT=$(pwd) && \

cd init && sudo docker compose up -d && \

sudo ln -sf $ROOT/init/animeenigma-nginx.conf /etc/nginx/sites-enabled/ && \
nginx -t && nginx -s reload && \

cd $ROOT/services/backend && \
npm ci && \
npm run build && \

cd $ROOT/services/frontend && \
npm ci && \
npm run build && \

pm2 restart $ROOT/init/pm2.config.cjs
