import { FiltersController } from './filters.controller'
import { FiltersService } from './filters.service'
import { Module } from '@nestjs/common'
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from '../caches/caches.module'
import { AnimeEntity } from 'src/anime/entity/anime.entity'
import { GenresEntity } from '../genres/entity/genres.entity'

@Module({
  imports: [
    CachesModule,
    TypeOrmModule.forFeature([AnimeEntity, GenresEntity])
  ],
  controllers: [FiltersController],
  providers: [FiltersService],
})
export class FilterModule {}
