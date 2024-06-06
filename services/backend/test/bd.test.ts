
import { Test, TestingModule } from '@nestjs/testing';
import { TypeOrmModule } from '@nestjs/typeorm';
import { config } from '../src/config/index';
import { join } from 'path';

import { AnimeCollectionsModule } from '../src/animeCollections/animeCollections.module'
import { AnimeCollectionsService } from '../src/animeCollections/animeCollections.service'

describe('DBService', () => {
  let animeCollectionsService: AnimeCollectionsService;

  beforeEach(async () => {
    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [
        AnimeCollectionsModule,
        TypeOrmModule.forRootAsync({
        useFactory: () => ({
          type: "postgres",
          host: config.pgHost,
          port: config.pgPort,
          username: config.pgUser,
          password: config.pgSecret,
          database: config.pgDB,
          synchronize: true,
          entities: [join(__dirname, '..', 'src') + '/**/*.entity{.js, .ts}']
        })
      })],
    }).compile();

    animeCollectionsService = moduleRef.get<AnimeCollectionsService>(AnimeCollectionsService);
  });

  test('TODO', async () => {

    // const data = await animeCollectionsService.findAll();

    expect(true).toBe(true);
  });
});
