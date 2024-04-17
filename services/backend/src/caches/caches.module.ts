import { redisStore } from 'cache-manager-redis-yet';
import { Module } from '@nestjs/common';
import { CacheModule } from '@nestjs/cache-manager';
import { CachesService } from './caches.service';
import { config } from '../config/index'

@Module({
  imports: [
    CacheModule.register({
      store : redisStore,
      ttl : 0,
      password : config.redisSecret,
      socket: {
        host: config.redisHost,
        port: config.redisPort
      }
    })
  ],
  exports: [CachesService],
  providers: [CachesService],
})

export class CachesModule {}
