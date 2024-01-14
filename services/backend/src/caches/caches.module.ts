import type { RedisClientOptions } from 'redis';
import { redisStore } from 'cache-manager-redis-yet';
import { Module } from '@nestjs/common';
import { CacheModule, CacheStore } from '@nestjs/cache-manager';
import { CachesService } from './caches.service';

@Module({
  imports: [
    CacheModule.register({
      store : redisStore,
      ttl : 0,
      host : 'localhost',
      port : 6379,
      password : "v0RedisSecret"
    })
  ],
  exports: [CachesService],
  providers: [CachesService],
})
export class CachesModule {}
