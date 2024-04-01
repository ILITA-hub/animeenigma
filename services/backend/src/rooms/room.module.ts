
import { Module } from '@nestjs/common';
import { AppController } from './room.controller';
import { RoomService } from './room.service';
// import { RoomGateway } from './room.gateway';
import { CachesModule} from '../caches/caches.module'
import { UserModule } from '../users/user.module';
import { TypeOrmModule } from '@nestjs/typeorm'
import { RoomEntity } from './entity/room.entity'
import { CryptoService } from "../crypto/crypto.sevice"
import { CryptoModule } from 'src/crypto/crypto.module';

@Module({
  imports: [CachesModule, UserModule, TypeOrmModule.forFeature([RoomEntity]), CryptoModule],
  controllers: [AppController],
  providers: [RoomService], // RoomGateway
})
export class RoomModule {}
