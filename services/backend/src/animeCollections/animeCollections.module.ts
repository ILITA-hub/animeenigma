import type { RedisClientOptions } from 'redis'
import { redisStore } from 'cache-manager-redis-yet'
import { Module } from '@nestjs/common'
import { CacheModule, CacheStore } from '@nestjs/cache-manager'
import { AnimeCollectionsController } from './animeCollections.controller'
import { AnimeCollectionsService } from './animeCollections.service'
import { AnimeCollections } from './entity/animeCollection.entity'
import { AnimeCollectionOpenings } from './entity/animeCollectionsOpenings.entity'
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from '../caches/caches.module'
import { UserEntity } from '../users/entity/user.entity'
import { VideosEntity } from '../videos/entity/videos.entity'


@Module({
  imports: [
    CachesModule,
    TypeOrmModule.forFeature([AnimeCollections, AnimeCollectionOpenings, UserEntity, VideosEntity])
  ],
  controllers: [AnimeCollectionsController],
  providers: [AnimeCollectionsService],
})
export class AnimeCollectionsModule {}
