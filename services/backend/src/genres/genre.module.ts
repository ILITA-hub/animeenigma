
import { Module } from '@nestjs/common';
import { GenreController } from './genre.controller';
import { GenreService } from './genre.service';
import { CachesModule} from '../caches/caches.module'
import { TypeOrmModule } from '@nestjs/typeorm'
import { GenresEntity } from './entity/genres.entity'

@Module({
  imports: [
    TypeOrmModule.forFeature([GenresEntity]), CachesModule
  ],
  controllers: [GenreController],
  providers: [GenreService],
})

export class GenreModule {}
