
const path = require('path');
const dir = path.normalize(path.join(path.dirname(module.filename), '..'));

module.exports = {
  apps: [
    {
      name: 'webBackend',
      script: path.join(dir, './backend/app.js'),
      watch: false,
      wait_ready: true,
      instances: 2,
      kill_timeout: 60000,
      env: {
        NODE_ENV: 'production',
      }
    },
    {
      name: 'webSocketServer',
      script: path.join(dir, './backend/webSockets/webSocket.js'),
      watch: false,
      wait_ready: true,
      instances: 2,
      kill_timeout: 60000,
      env: {
        NODE_ENV: 'production',
      }
    },
    {
      name: 'webFrontendServe',
      script: 'serve',
      exec_mode: 'cluster',
      instances: 2,
      wait_ready: true,
      kill_timeout: 120000,
      watch: false,
      cwd: dir,
      env: {
        PM2_SERVE_PATH: path.join(dir, './frontend/dist'),
        PM2_SERVE_PORT: 8080,
        PM2_SERVE_SPA: 'true',
        PM2_SERVE_HOMEPAGE: '/index.html'
      }
    }
  ]
};
