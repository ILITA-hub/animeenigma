
import { createClient } from 'redis';
import config from '../../config/index.js';

const client = createClient({
    password: config.redisSecret,
    host: config.redisHost,
    port: config.redisPort
});

client.on('error', function (error) {
    console.error(error);
});

client.connect();

const client2 = {
    get: async (key) => {
        return JSON.parse(await client.get(key));
    },
    set: async (key, value) => {
        await client.set(key, JSON.stringify(value));
    },
    del: async (key) => {
        await client.del(key);
    },
}

export default client2;
