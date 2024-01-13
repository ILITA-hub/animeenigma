
# we in the root of the project

$ROOT << $(pwd)

cd backend && npm install
cd ../frontend && npm install && npm run build

pm2 kill
pm2 start ./init/pm2.config.js

docker compose up -d

# sudo ln -s /home/nandi/data/animeenigma/init/nginx.conf /etc/nginx/sites-enabled
nginx -t && nginx -s reload
