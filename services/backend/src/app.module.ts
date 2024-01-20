import { Module } from '@nestjs/common';
import { RoomModule } from './rooms/room.module'
import { AnimeCollectionsModule } from './animeCollections/animeCollections.module'
import { Logger } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from './caches/caches.module'
import { UserModule } from './users/user.module'

@Module({
  imports: [
    RoomModule,
    CachesModule,
    AnimeCollectionsModule,
    TypeOrmModule.forRootAsync({
      useFactory: () => ({
        type: "postgres",
        host: "localhost",
        port: 5432,
        username: "postgresUserAE",
        password: "pgSuperSecretMnogaBycaBab",
        database: "postgres",
        synchronize: true,
        entities: [__dirname + '/**/*.entity{.js, .ts}']
      })
    }),
    UserModule,
  ],
  providers: [Logger],
})
export class AppModule {}
