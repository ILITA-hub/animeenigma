import { createClient } from 'redis';
import config from './config.tmpl.cjs';

let client;

async function init() {
    client = createClient({
        // password: config.redisSecret,
        url: `redis://:${config.redisSecret}@localhost:${config.redisPort}`
    }).on('error', err => console.log('Redis Client Error', err))

    await client.connect();

    // await client.set('key', 'value');
    // const value = await client.get('key');
    // await client.disconnect();

}

await init();

async function setCache(key, value) {
    await client.set(key, JSON.stringify(value));
}
async function getCache(key) {
    return JSON.parse(await client.get(key));
}
async function delCache(key) {
    await client.del(key);
}

export { setCache, getCache, delCache }
