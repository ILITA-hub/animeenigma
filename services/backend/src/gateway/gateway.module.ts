import { Module } from '@nestjs/common'
import { MyGateway } from './geteway';

@Module({
    providers: [ MyGateway ]
})
export class GatewayModule {}