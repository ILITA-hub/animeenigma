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
    const roomsBacend = await axios.get('http://0.0.0.0:1000/rooms')
    const roomsKey = []

    for (let key in roomsBacend['data']) {
      roomsKey.push(key)
    }

    // const rooms = await this.RoomRepository.createQueryBuilder('room')
    //   .select(["room.id", "room.name", "room.maxPlayer", "room.status", "room.uniqueURL"])
    //   .where("room.status != :status", {status: RoomStatus.CREATING})
    //   .andWhere("room.uniqueURL = ANY(:ids::text[])", {ids: roomsKey})

    const rooms = await this.RoomOpRepository.query(
      `SELECT *
      FROM room 
      WHERE room."uniqueURL" = ANY($1::text[])`,
      [roomsKey]
    )

    return rooms
  }

  async updateRoom(id: string, room: Room): Promise<void> {
    await this.cachesService.setCache('room' + id, room);
  }

  async createRoom(body: SchemaRoom) {
    const roomID = roomIdGenerate()

    const openings = []
    const videosIds = []

    for (let i = 0; i < body.rangeOpenings.length; i++) {
      const range = body.rangeOpenings[i]

      switch(range.type) {
        case "video":
          openings.push(range.id)
          break

        case "collection":
          videosIds.push(range.id)
          break
      }
    }

    const videoIdsCollection = await this.animeCollectionsOpeningRepository.query(`SELECT videos."id" as "videoId"  from "animeCollections"
      join "animeCollectionOpenings" on "animeCollectionOpenings"."animeCollectionId" = "animeCollections".id 
      join videos ON videos.id = "animeCollectionOpenings"."animeOpeningId"
      where "animeCollections".id = ANY($1::int[])
      group by videos."id"`, [videosIds])

    openings.push(...videoIdsCollection.map(value => {
      return value['videoId']
    }))

    await axios.post('http://0.0.0.0:1000/create_room', {
      name: body.name,
      maxPlayer: body.qtiUsersMax,
      uniqueURL: roomID,
      status: RoomStatus.CREATING,
      openingsType: body.rangeOpenings,
      openings: openings
    })

    return {
      roomId: roomID
    }
  }
}
