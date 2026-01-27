import { Inject, Injectable } from '@nestjs/common';
import { CACHE_MANAGER } from '@nestjs/cache-manager';
import { Cache } from 'cache-manager';

@Injectable()
export class CachesService {

  constructor(@Inject(CACHE_MANAGER) private cacheManager: Cache) {}

  async setCache(key: string, value: any, ttl?: number) {
    if (ttl) {
      await this.cacheManager.set(key, value, ttl * 1000); // cache-manager expects ms
    } else {
      await this.cacheManager.set(key, value);
    }
  }
  async getCache(key: string): Promise<any> {
    return await this.cacheManager.get(key);
  }
  async delCache(key: string) {
    await this.cacheManager.del(key);
  }

}
