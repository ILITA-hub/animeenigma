import { Module } from '@nestjs/common';
import { RoomModule } from './rooms/room.module'
import { AnimeCollectionsModule } from './animeCollections/animeCollections.module'
import { Logger } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from './caches/caches.module'
import { UserModule } from './users/user.module'
import { AnimeModule } from './anime/anime.module'
import { config } from './config/index'
import { ServeStaticModule } from '@nestjs/serve-static';
import { GenreModule } from './genres/genre.module'
import { VideosModule } from './videos/videos.module'
import { FilterModule } from './filter/filters.module'
import { ShikimoriModule } from './shikimori/shikimori.module'
import { StreamingModule } from './streaming/streaming.module'
import { join } from 'path';


@Module({
  imports: [
    AnimeModule,
    RoomModule,
    CachesModule,
    AnimeCollectionsModule,
    GenreModule,
    VideosModule,
    FilterModule,
    ShikimoriModule,
    StreamingModule,
    TypeOrmModule.forRootAsync({
      useFactory: () => ({
        type: "postgres",
        host: config.pgHost,
        port: config.pgPort,
        username: config.pgUser,
        password: config.pgSecret,
        database: config.pgDB,
        synchronize: true,
        entities: [__dirname + '/**/*.entity{.js, .ts}']
      })
    }),
    UserModule,
    ServeStaticModule.forRoot({
      rootPath: join(__dirname, '..', '..', '..', 'animeResources'),
    }),
  ],
  providers: [Logger],
})
export class AppModule {}
