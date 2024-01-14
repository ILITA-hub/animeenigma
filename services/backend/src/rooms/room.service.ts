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
    return rooms;
  }

  async getRoom(id: string) {
    let result = {
      status: 200,
      room: Room 
    }
    const room = await this.cachesService.getCache(`room${id}`);
    if (!room) {
      result.status = 404
      return result
    }
    result.room = room;
    return result;
  }

  async createRoom(body: SchemaRoom) {
    let result = {
      status : 200
    }
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
    return result
  }
}
