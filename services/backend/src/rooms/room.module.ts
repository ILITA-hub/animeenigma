
import { Module } from '@nestjs/common';
import { AppController } from './room.controller';
import { RoomService } from './room.service';
import { RoomGateway } from './room.gateway';
import { CachesModule} from '../caches/caches.module'
import { UserModule } from '../users/user.module';

@Module({
  imports: [CachesModule, UserModule],
  controllers: [AppController],
  providers: [RoomService, RoomGateway],
})
export class RoomModule {}
