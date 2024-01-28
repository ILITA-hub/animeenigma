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
import { UserService } from '../users/user.service';
import { CachesService } from '../caches/caches.service';

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
    private readonly userService: UserService,
    private readonly cachesService: CachesService,
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


  // @UseGuards(SocketAuthGuard)
  @SubscribeMessage('joinRoom')
  async userConnectedToRoom(
    @Request() req,
    @MessageBody() data: any,
    @ConnectedSocket() socket: Socket,
  ): Promise<void> {

    const client = this.clients[socket.id];
    const roomId = data.roomId;
    const user = this.clients[client.id]['user'];


    const room = await this.roomService.getRoom(roomId);

    if (!room) {
      client.emit('roomNotFound', 'roomNotFound');
      console.log('room  NE CYWECTBYUET')
      return;
    }

    client['roomId'] = roomId;
    room.users[client.id] = user;

    await this.cachesService.setCache('room' + roomId, room);

    this.broadcastMessage('roomUpdate', room);
  }


  async afterInit(socket: Socket) {
    this.clients = new Clients();

    // const startWorld = await this.worldStateService.getLast();
    // this.worldService.setCache('world', startWorld);

    this.logger.log('WS Init');
  }


  async handleDisconnect(socket: Socket) {

    const client = this.clients[socket.id];

    if (!client) {
      this.logger.warn(`No client to disconnect: ${socket.id}`);
      return;
    }

    const roomId = client['roomId'];

    if (roomId) {
      const room = await this.cachesService.getCache('room' + roomId);
      if (room?.users?.[client.id]) {
        delete room.users[client.id];
        await this.cachesService.setCache('room' + roomId, room);
        this.broadcastMessage('roomUpdate', room);
      }
    }
    
    delete this.clients[client.id];
    this.logger.log(`Client disconnected: ${client.id}`);
  }


  async handleConnection(
    client: Socket,
    ...args: any[]
  ): Promise<void> {
    this.logger.log(`Client connection tried: ${client.id}`);

    // TODO make auth guard
    const sessionId = String(client.handshake.query.sessionId);

    if (!sessionId) {
      client.emit('login', 'unauthorized');
      client.disconnect(true);
      return;
    }
    console.log(sessionId)
    const userSession = await this.userService.getUserSession(sessionId); 
    
    if (!userSession) {
      client.emit('login', 'unauthorized');
      client.disconnect(true);
      return;
    }

    const user = await this.userService.getUserById(userSession.userId);
    client['user'] = user;

    this.clients[client.id] = client;
    
    this.logger.log(`Client connected: ${client.id}`);

    this.broadcastMessage('user connected', { userName: user.name });
  }
  

  broadcastMessage(event: string, payload: any) {
    for (const client of this.clients) {
      client.emit(event, payload);
    }
  }
}