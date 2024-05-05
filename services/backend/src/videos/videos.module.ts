
import { Module } from '@nestjs/common';
import { VideosController } from './videos.controller';
import { VideosService } from './videos.service';
import { CachesModule} from '../caches/caches.module'
import { TypeOrmModule } from '@nestjs/typeorm'
import { VideosEntity } from './entity/videos.entity'
import { AnimeEntity } from '../anime/entity/anime.entity'

@Module({
  imports: [
    TypeOrmModule.forFeature([AnimeEntity]),
    TypeOrmModule.forFeature([VideosEntity]),
  ],
  controllers: [VideosController],
  providers: [VideosService],
})

export class VideosModule {}
