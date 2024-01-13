import {
  SubscribeMessage,
  WebSocketGateway,
  OnGatewayInit,
  WebSocketServer,
  OnGatewayConnection,
  OnGatewayDisconnect,
  MessageBody,
  ConnectedSocket,
} from '@nestjs/websockets';
import { Logger, UseGuards, Request } from '@nestjs/common';
import { Socket } from 'socket.io';
import { RoomService } from './room.service';
import { SocketAuthGuard } from '../auth/guards/socket.guard';
import { Clients } from './dto/TEMP-clients.dto' // УБРАТЬ ЭТО ГОВНО И СДЕЛАТЬ НОРМАЛЬНО (через кеши)

@WebSocketGateway(9000, {
  transports: ['websocket'],
  // allowEIO3: true,
  // cors: {
  //   origin: true,
  //   credentials: true
  // },
})
export class RoomGateway implements OnGatewayInit, OnGatewayConnection, OnGatewayDisconnect {
  constructor(
    private readonly roomService: RoomService,
  ) {}
  @WebSocketServer() 
  clients: Clients;
  server: Socket;

  private logger: Logger = new Logger('RoomsGateway');


  @UseGuards(SocketAuthGuard)
  @SubscribeMessage('chatMessage')
  chatMessageHandle(
    @Request() req,
    @MessageBody() data: any,
    @ConnectedSocket() client: Socket,
  ): void {
    data.text = req.user.username + ': ' + data.text;
    this.broadcastMessage('chatMessage', data);
  }


  @UseGuards(SocketAuthGuard)
  @SubscribeMessage('message')
  async handleMessage2(
    @Request() req,
    @MessageBody() data: any,
    @ConnectedSocket() client: Socket,
  ): Promise<void> {

    // let world = await this.worldService.getCache('world');

    // world.data = this.worldService.testWorldPayloadProcess(req.user, world.data, data);

    // this.worldService.setCache('world', world);
    this.broadcastMessage('message', data);

  }


  async afterInit(socket: Socket) {
    this.clients = new Clients();

    // const startWorld = await this.worldStateService.getLast();
    // this.worldService.setCache('world', startWorld);

    this.logger.log('WS Init');
  }


  handleDisconnect(client: Socket) {
    this.clients[client.id] = undefined;
    this.logger.log(`Client disconnected: ${client.id}`);
  }


  async handleConnection(client: Socket, ...args: any[]) {
    client.send('hi', 'hi');
    this.clients[client.id] = client;
    this.logger.log(`Client connected: ${client.id}`);

    this.broadcastMessage('world', {});
  }
  

  broadcastMessage(event: string, payload: any) {
    for (const client of this.clients) {
      client.emit(event, payload);
    }
  }
}