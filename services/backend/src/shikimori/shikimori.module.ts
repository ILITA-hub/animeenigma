import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { ShikimoriController } from './shikimori.controller';
import { ShikimoriService } from './shikimori.service';
import { ShikimoriAnime } from './entity/shikimori-anime.entity';
import { AnimeEntity } from '../anime/entity/anime.entity';
import { CachesModule } from '../caches/caches.module';
import { AuthModule } from '../auth/auth.module';

@Module({
  imports: [
    TypeOrmModule.forFeature([ShikimoriAnime, AnimeEntity]),
    CachesModule,
    AuthModule,
  ],
  controllers: [ShikimoriController],
  providers: [ShikimoriService],
  exports: [ShikimoriService],
})
export class ShikimoriModule {}
