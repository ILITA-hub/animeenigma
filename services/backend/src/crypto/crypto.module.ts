import type { RedisClientOptions } from 'redis';
import { redisStore } from 'cache-manager-redis-yet';
import { Module } from '@nestjs/common';
import { CryptoService } from './crypto.sevice';

@Module({
  imports: [],
  exports: [CryptoService],
  providers: [CryptoService],
})
export class CryptoModule {}
