import { Module } from '@nestjs/common';
import { AnimeController } from './anime.controller';
import { AnimeService } from './anime.service';
import { TypeOrmModule } from '@nestjs/typeorm'
import { AnimeEntity } from './entity/anime.entity'

@Module({
  imports: [
    TypeOrmModule.forFeature([AnimeEntity])
  ],
  controllers: [AnimeController],
  providers: [AnimeService],
  exports: [AnimeService],
})

export class AnimeModule {}
