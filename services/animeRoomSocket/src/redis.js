
import { createClient } from 'redis';
import { config } from '../index.js';

let client;

async function init() {
    client = await createClient({
        password: config.redisSecret
    })
        .on('error', err => console.log('Redis Client Error', err))
        .connect();

    // await client.set('key', 'value');
    // const value = await client.get('key');
    // await client.disconnect();

}

init();

async function setCache(key, value) {
    await client.set(key, value);
}
async function getCache(key) {
    return JSON.parse(await client.get(key));
}
async function delCache(key) {
    await client.del(key);
}

export { setCache, getCache, delCache }
