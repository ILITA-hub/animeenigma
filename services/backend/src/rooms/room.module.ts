import type { RedisClientOptions } from 'redis';
import { redisStore } from 'cache-manager-redis-yet';
import { Module } from '@nestjs/common';
import { CacheModule, CacheStore } from '@nestjs/cache-manager';
import { AppController } from './room.controller';
import { RoomService } from './room.service';
import { RoomGateway } from './room.gateway';

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
  controllers: [AppController],
  providers: [RoomService, RoomGateway],
})
export class RoomModule {}
