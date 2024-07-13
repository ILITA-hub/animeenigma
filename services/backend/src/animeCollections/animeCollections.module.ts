
import { Module } from '@nestjs/common'
import { AnimeCollectionsController } from './animeCollections.controller'
import { AnimeCollectionsService } from './animeCollections.service'
import { AnimeCollections } from './entity/animeCollection.entity'
import { AnimeCollectionOpenings } from './entity/animeCollectionsOpenings.entity'
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from '../caches/caches.module'
import { UserEntity } from '../users/entity/user.entity'
import { VideosEntity } from '../videos/entity/videos.entity'
import { AnimeEntity } from 'src/anime/entity/anime.entity'


@Module({
  imports: [
    CachesModule,
    TypeOrmModule.forFeature([AnimeCollections, AnimeCollectionOpenings, UserEntity, VideosEntity, AnimeEntity])
  ],
  controllers: [AnimeCollectionsController],
  providers: [AnimeCollectionsService],
})
export class AnimeCollectionsModule {}
