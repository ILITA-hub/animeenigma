import { Injectable } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm'
import { generateRoomId } from '../utils/miscellaneous'
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { QueryFailedError, Repository } from 'typeorm'
import { CachesService } from '../caches/caches.service'
import { exec } from 'child_process'
import { RoomEntity, RoomOpeningsEntity, RoomStatus } from './entity/room.entity'
import { Cron } from '@nestjs/schedule'
import { getCiphers } from "crypto"
import { CryptoService } from "../crypto/crypto.sevice"
import { roomIdGenerate } from "../utils/miscellaneous"

let roomPort = 10000

@Injectable()
export class RoomService {
  

  constructor(
    @InjectRepository(RoomEntity) private readonly RoomRepository: Repository<RoomEntity>,
    @InjectRepository(RoomOpeningsEntity) private readonly RoomOpRepository: Repository<RoomOpeningsEntity>,
    private readonly cachesService: CachesService,
    private readonly CryptoService: CryptoService,
  ) { }

  async getAllRooms() {
    return await this.RoomRepository.find()
  }

  async updateRoom(id: string, room: Room): Promise<void> {
    await this.cachesService.setCache('room' + id, room);
  }

  async deleteAll() {
    
  }

  async createRoom(body: SchemaRoom) {
    const ports = await this.RoomRepository.find({
      where: {
        deleteAt: null
      },
      order: {
        port: "ASC"
      }
    })

    let port = 10000

    for(let i = 0; i < ports.length; i++) {
      if (port == ports[i].port) {
        port++
        continue
      }
      break
    }

    const roomID = roomIdGenerate()

    const room = await this.RoomRepository.save({
      name: body.name,
      maxPlayer: body.qtiUsersMax,
      port: port,
      uniqueURL: roomID,
      status: RoomStatus.STARTING
    })

    for(let i = 0; i < body.rangeOpenings.length; i++) {
      const range = body.rangeOpenings[i]
      await this.RoomOpRepository.save({
        idRoom: room.id,
        type: range.type,
        idEntity: range.id
      })
    }

    exec(`cd ../animeRoomSocket/; PORT=${port} ID=${room.id} npm run start`) // todo - вынести в отдельного воркера
    console.log(`cd ../animeRoomSocket/; PORT=${port} ID=${room.id} npm run start`)

    return roomID
  }

  async deleteRoom(id: string): Promise<void> {
    const room = await this.RoomRepository.findOne({
      where: {
        deleteAt: null,
        uniqueURL: id
      }
    })

    exec(`kill $(lsof -t -i:${room.port})`)

    await this.RoomRepository.update(room.id, {
      deleteAt: new Date()
    })
  }

  @Cron('0 * * * *')
  async roomWatcher() {
    const allRooms = await this.cachesService.getCache("rooms");
  }
}
