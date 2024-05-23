
import { Test, TestingModule } from '@nestjs/testing';
import { CachesModule } from './../src/caches/caches.module';
import { CachesService } from './../src/caches/caches.service';

describe('CachesService', () => {
  let cachesService: CachesService;

  beforeEach(async () => {
    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [CachesModule],
    }).compile();

    cachesService = moduleRef.get<CachesService>(CachesService);
  });

  test('Send and retrive "123" from caches', async () => {

    const key = 'test_caches';
    const value = 123;

    await cachesService.setCache(key, value)
    const retrivedValue = await cachesService.getCache(key)
    await cachesService.delCache(key)

    expect(retrivedValue).toBe(value);
  });
});
