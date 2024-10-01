import { Injectable } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm'
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { Repository } from 'typeorm'
import { CachesService } from '../caches/caches.service'
import { RoomEntity, RoomOpeningsEntity, RoomStatus } from './entity/room.entity'
import { AnimeCollectionOpenings } from '../animeCollections/entity/animeCollectionsOpenings.entity'
import { VideosEntity } from '../videos/entity/videos.entity'
import { roomIdGenerate } from "../utils/miscellaneous"
import axios from 'axios';

@Injectable()
export class RoomService {


  constructor(
    @InjectRepository(RoomEntity) private readonly RoomRepository: Repository<RoomEntity>,
    @InjectRepository(RoomOpeningsEntity) private readonly RoomOpRepository: Repository<RoomOpeningsEntity>,
    @InjectRepository(AnimeCollectionOpenings) private readonly animeCollectionsOpeningRepository: Repository<AnimeCollectionOpenings>,
    private readonly cachesService: CachesService,
  ) { }

  async getAllRooms() {
    const rooms = await this.RoomRepository.createQueryBuilder('room')
      .select(["room.id", "room.name", "room.maxPlayer", "room.status", "room.uniqueURL"])
      .where("room.status != :status", {status: RoomStatus.CREATING})
      .getMany()

    const roomsBacend = await axios.get('http://0.0.0.0:1000/rooms')
    console.log(roomsBacend)

    return rooms
  }

  async updateRoom(id: string, room: Room): Promise<void> {
    await this.cachesService.setCache('room' + id, room);
  }

  async createRoom(body: SchemaRoom) {
    const roomID = roomIdGenerate()

    const openings = []

    for (let i = 0; i < body.rangeOpenings.length; i++) {
      const range = body.rangeOpenings[i]

      switch(range.type) {
        case "video":
          openings.push(range.id)
          break

        case "collection":
          const videos = await this.animeCollectionsOpeningRepository.createQueryBuilder('animeCollectionOpenings')
            .where("animeCollectionOpenings.animeCollectionId = :id", {id: range.id})
            .select(["animeCollectionOpenings.animeOpeningId"])
            .getRawMany()

          const videosId = videos.map(value => value.animeOpeningId)
          openings.push(...videosId)
          break
      }
    }

    await axios.post('http://0.0.0.0:1000/create_room', {
      name: body.name,
      maxPlayer: body.qtiUsersMax,
      uniqueURL: roomID,
      status: RoomStatus.CREATING,
      openings: openings
    })

    return roomID
  }
}
