import { Injectable } from '@nestjs/common';
import { generateRoomId } from '../utils/miscellaneous'
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { CachesService } from '../caches/caches.service'

@Injectable()
export class RoomService {
  constructor(
    private readonly cachesService: CachesService,
  ) {}

  async getAllRooms() {
    const rooms = await this.cachesService.getCache("rooms");
    let result = []
    for(let el of rooms ? rooms : []) {
      const room = await this.cachesService.getCache(`room${el}`);
      result.push(room)
    }
    return result;
  }

  async getRoom(id: string): Promise<Room> {
    const room = await this.cachesService.getCache(`room${id}`);

    console.log({room})

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

  async createRoom(body: SchemaRoom) {
    const roomId = generateRoomId();
    const newRoom = new Room(roomId, body.name, body.ownerId, body.rangeOpenings);
    await this.cachesService.setCache(`room${roomId}`, newRoom);

    const allRooms = await this.cachesService.getCache("rooms");
    if (!allRooms) {
      await this.cachesService.setCache("rooms", [roomId])
    } else {
      await this.cachesService.setCache("rooms", [...allRooms, roomId])
    }
    return roomId;
  }

  async deleteRoom(id: string) {
    let result = {
      status : 200
    }
    const roomId = id

    const roomToDelete = await this.cachesService.getCache(`room${roomId}`)
    if (!roomToDelete) {
      result.status = 403;
      return result
    }
    await this.cachesService.delCache(`room${roomId}`)

    const allRooms = await this.cachesService.getCache("rooms");
    const arr = allRooms.filter((word) => word != roomId);
    await this.cachesService.setCache("rooms", [...arr])
    
    return result
  }
}
