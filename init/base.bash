
cd backend && npm install
cd ../frontend && npm install

docker compose up -d

nginx -s reload
