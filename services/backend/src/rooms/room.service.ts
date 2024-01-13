import { Inject, Injectable } from '@nestjs/common';
import { CacheTTL, CACHE_MANAGER } from '@nestjs/cache-manager';
import { Cache } from 'cache-manager';
import { generateRoomId } from '../utils/miscellaneous'
import { Room } from './dto/create-room.dto'

@Injectable()
export class RoomService {

  constructor(@Inject(CACHE_MANAGER) private cacheManager: Cache) {}

  async setCache(key: string, value: any) {
    await this.cacheManager.set(key, value);
  }
  async getCache(key: string): Promise<any> {
    return await this.cacheManager.get(key);
  }
  async delCache(key: string) {
    await this.cacheManager.del(key);
  }

  async getAllRooms() {
    const rooms = await this.getCache("rooms");
    return rooms;
  }

  async getRoom(id: string) {
    const room = await this.getCache(`room${id}`);
    return room;
  }

  async createRoom(body: Room) {
    let result = {
      status : 200
    }
    const roomId = generateRoomId();
    const newRoom = new Room(roomId, body.name, body.ownerId, body.rangeAnime);
    await this.setCache(`room${roomId}`, newRoom);

    const allRooms = await this.getCache("rooms");
    if (!allRooms) {
      await this.setCache("rooms", [roomId])
    } else {
      await this.setCache("rooms", [...allRooms, roomId])
    }
    return roomId;
  }

  async deleteRoom(id: string) {
    let result = {
      status : 200
    }
    const roomId = id

    const roomToDelete = await this.getCache(`room${roomId}`)
    if (!roomToDelete) {
      result.status = 403;
      return result
    }
    await this.delCache(`room${roomId}`)
    return result
  }
}
