
import { createRequire } from 'module';
const require = createRequire(import.meta.url);

const defaultConfig = require('./config.tmpl.cjs');

let localConfig = {};
try {
  localConfig = require('./config.cjs');
} catch (error) {
  console.log('Local config not found, using tmpl config');
}

const exportsConfig = localConfig ? Object.assign(defaultConfig, localConfig) : defaultConfig;

export default exportsConfig;
