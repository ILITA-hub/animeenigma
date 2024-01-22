import { Inject, Injectable } from '@nestjs/common';
import { CACHE_MANAGER } from '@nestjs/cache-manager';
import { Cache } from 'cache-manager';
import { generateRoomId } from '../utils/miscellaneous'

@Injectable()
export class CachesService {

  constructor(@Inject(CACHE_MANAGER) private cacheManager: Cache) {}

  async setCache(key: string, value: any) {
    await this.cacheManager.set(key, value);
  }
  async getCache(key: string): Promise<any> {
    return await this.cacheManager.get(key);
  }
  async delCache(key: string) {
    await this.cacheManager.del(key);
  }

}
