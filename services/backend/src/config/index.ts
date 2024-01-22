
import { createRequire } from 'module';
import { config as defaultConfig } from './config.tmpl';

let localConfig = {};
try {
  localConfig = require('./config.cjs');
} catch (error) {
  console.log('Local config not found, using tmpl config');
}

const exportsConfig = localConfig ? Object.assign(defaultConfig, localConfig) : defaultConfig;

export const config = exportsConfig;
