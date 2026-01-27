const config = {

    redisSecret: 'v0RedisSecret',
    redisHost: '127.0.0.1',
    redisPort: 16379,

    restPort: 13000,

    webSocketPort: 19000,

    pgHost: '127.0.0.1',
    pgPort: 15432,
    pgUser: 'postgresUserAE',
    pgSecret: 'pgSuperSecretMnogaBycaBab',
    pgDB: 'animeenigma',

    jwtSecret: 'jwtSecret',

    // MinIO Configuration (self-hosted S3-compatible storage)
    minio: {
        endPoint: '127.0.0.1',
        port: 9000,
        useSSL: false,
        accessKey: 'minioadmin',
        secretKey: 'minioadmin',
        bucket: 'anime-videos',
        publicUrl: 'http://localhost:9000', // URL for direct access
    },

    // Shikimori API Configuration
    shikimori: {
        baseUrl: 'https://shikimori.one/api',
        userAgent: 'AnimeEnigma', // Required by Shikimori API
        rateLimit: 5, // requests per second
        cacheTTL: 3600, // cache duration in seconds
    },

    // Streaming Configuration
    streaming: {
        // Chunk size for video streaming (2MB)
        chunkSize: 2 * 1024 * 1024,
        // Maximum buffer size
        maxBufferSize: 10 * 1024 * 1024,
        // Timeout for external API requests (30 seconds)
        externalTimeout: 30000,
        // Enable proxy/restreaming for external sources
        enableProxy: true,
    },

    // External Streaming APIs (examples - configure as needed)
    externalApis: {
        // Kodik API (example)
        kodik: {
            baseUrl: 'https://kodikapi.com',
            apiKey: '', // Add your API key
            enabled: false,
        },
        // Anilibria API (example)
        anilibria: {
            baseUrl: 'https://api.anilibria.tv/v3',
            enabled: false,
        },
    },

}

export { config };
