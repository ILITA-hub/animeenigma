import { Module } from '@nestjs/common';
import { AppController } from './room.controller';
import { RoomService } from './room.service';
import { CachesModule} from '../caches/caches.module'
import { UserModule } from '../users/user.module';
import { TypeOrmModule } from '@nestjs/typeorm'
import { RoomEntity, RoomOpeningsEntity } from './entity/room.entity'
import { CryptoModule } from 'src/crypto/crypto.module';
import { AnimeCollectionOpenings } from '../animeCollections/entity/animeCollectionsOpenings.entity'

@Module({
  imports: [CachesModule, UserModule, TypeOrmModule.forFeature([RoomEntity, RoomOpeningsEntity, AnimeCollectionOpenings]), CryptoModule],
  controllers: [AppController],
  providers: [RoomService],
})
export class RoomModule {}
