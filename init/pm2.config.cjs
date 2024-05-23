
const path = require('path');
const dir = path.normalize(path.join(path.dirname(module.filename), '..'));

module.exports = {
  apps: [
    {
      name: 'animeenigmaBackend',
      script: path.join(dir, './services/backend/dist/main.js'),
      watch: false,
      wait_ready: true,
      // instances: 2,
      kill_timeout: 60000,
      env: {
        NODE_ENV: 'production',
      }
    },
    {
      name: 'animeenigmaFrontendServe',
      script: 'serve',
      exec_mode: 'cluster',
      instances: 2,
      wait_ready: true,
      kill_timeout: 120000,
      watch: false,
      cwd: dir,
      env: {
        PM2_SERVE_PATH: path.join(dir, './services/frontend/dist'),
        PM2_SERVE_PORT: 18080,
        PM2_SERVE_SPA: 'true',
        PM2_SERVE_HOMEPAGE: '/index.html'
      }
    }
  ]
};
