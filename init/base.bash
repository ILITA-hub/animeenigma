
# we in the root of the project

cd backend && npm install
cd ../frontend && npm install && npm run build

pm2 kill
pm2 start ./init/pm2.config.js

docker compose up -d

nginx -s reload
