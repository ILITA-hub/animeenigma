import { Injectable } from '@nestjs/common';
import { generateRoomId } from '../utils/miscellaneous'
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { CachesService } from '../caches/caches.service'
import { exec } from 'child_process'
import { Cron } from '@nestjs/schedule';

let roomPort = 10000

@Injectable()
export class RoomService {
  constructor(
    private readonly cachesService: CachesService,
  ) { }

  async getAllRooms() {
    const rooms = await this.cachesService.getCache("rooms");
    let result = []
    for (let el of rooms ? rooms : []) {
      const room = await this.cachesService.getCache(`room${el}`);
      result.push(room)
    }
    return result;
  }

  async getRoom(id: string): Promise<Room> {
    const room = await this.cachesService.getCache(`room${id}`);

    console.log({ room })

    return room;
  }

  async updateRoom(id: string, room: Room): Promise<void> {
    await this.cachesService.setCache('room' + id, room);
  }

  async deleteAll() {
    const rooms = await this.cachesService.getCache("rooms");
    for (let i = 0; i < rooms.length; i++) {
      const room = rooms[i]
      const room2 = await this.cachesService.getCache(`room${room}`);
      if (room2) {
        await this.cachesService.delCache(`room${room}`)
      }
    }
    await this.cachesService.delCache("rooms");
  }

  async createRoom(body: SchemaRoom): Promise<Room> {
    const roomId = generateRoomId();
    let ownerId = "0"

    const port = roomPort++ // TODO СДЕЛАТЬ НОРМАЛЬНО

    const newRoom = new Room(roomId, body.name, ownerId, body.rangeOpenings, port);
    await this.cachesService.setCache(`room${roomId}`, newRoom);

    const allRooms = await this.cachesService.getCache("rooms");
    if (!allRooms) {
      await this.cachesService.setCache("rooms", [roomId])
    } else {
      await this.cachesService.setCache("rooms", [...allRooms, roomId])
    }

    exec(`cd ../animeRoomSocket/; PORT=${port} ID=${roomId} npm run start`)
    // console.log(`cd ../animeRoomSocket/; PORT=${port} ID=${roomId} npm run start`)

    return newRoom;
  }

  async deleteRoom(id: string): Promise<void> {
    const roomId = id

    const roomToDelete = await this.cachesService.getCache(`room${roomId}`)
    if (!roomToDelete) {
      return
    }
    await this.cachesService.delCache(`room${roomId}`)

    const allRooms = await this.cachesService.getCache("rooms");
    const arr = allRooms.filter((word) => word != roomId);
    await this.cachesService.setCache("rooms", [...arr])
    exec(`kill $(lsof -t -i:${roomToDelete.port})`)
  }

  @Cron('0 * * * *')
  async roomWatcher() {
    const allRooms = await this.cachesService.getCache("rooms");
  }
}
