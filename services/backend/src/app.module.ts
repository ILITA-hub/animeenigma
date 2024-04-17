import { Module } from '@nestjs/common';
import { RoomModule } from './rooms/room.module'
import { AnimeCollectionsModule } from './animeCollections/animeCollections.module'
import { Logger } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from './caches/caches.module'
import { UserModule } from './users/user.module'
import { config } from './config/index'
import { ServeStaticModule } from '@nestjs/serve-static';

import { join } from 'path';

console.log(22)

@Module({
  imports: [
    RoomModule,
    CachesModule,
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
