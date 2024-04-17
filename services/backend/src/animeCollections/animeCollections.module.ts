
import { Module } from '@nestjs/common'
import { AnimeCollectionsController } from './animeCollections.controller'
import { AnimeCollectionsService } from './animeCollections.service'
import { AnimeCollections } from './entity/animeCollection.entity'
import { AnimeCollectionOpenings } from './entity/animeCollectionsOpenings.entity'
import { TypeOrmModule } from '@nestjs/typeorm'


@Module({
  imports: [
    TypeOrmModule.forFeature([AnimeCollections, AnimeCollectionOpenings])
  ],
  controllers: [AnimeCollectionsController],
  providers: [AnimeCollectionsService],
})
export class AnimeCollectionsModule {}
