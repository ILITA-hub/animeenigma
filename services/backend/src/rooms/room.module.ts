
import { Module } from '@nestjs/common';
import { AppController } from './room.controller';
import { RoomService } from './room.service';
import { RoomGateway } from './room.gateway';
import { CachesModule} from '../caches/caches.module'

@Module({
  imports: [CachesModule],
  controllers: [AppController],
  providers: [RoomService, RoomGateway],
})
export class RoomModule {}
