import { Module } from '@nestjs/common';
import { RoomModule } from './rooms/room.module'
import { animeCollectionsModule } from './animeCollections/animeCollections.module'
import { Logger } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm'
import { CachesModule } from './caches/caches.module'

@Module({
  imports: [
    RoomModule,
    CachesModule,
    animeCollectionsModule,
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
  ],
  providers: [Logger],
})
export class AppModule {}
