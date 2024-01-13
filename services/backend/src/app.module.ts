import { Module } from '@nestjs/common';
import { RoomModule } from './rooms/room.module'
import { Logger } from '@nestjs/common';
// import { GatewayModule } from './gateway/gateway.module'

@Module({
  imports: [RoomModule],
  providers: [Logger],
})
export class AppModule {}
