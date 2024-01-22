import { Injectable } from '@nestjs/common';
import { Cache } from 'cache-manager';
import { generateRoomId } from '../utils/miscellaneous'
import { Repository, createQueryBuilder, EntityManager } from 'typeorm';
import { InjectEntityManager, InjectRepository } from '@nestjs/typeorm'
import { AnimeCollections } from './entity/animeCollection.entity'
import { AnimeCollectionOpenings } from './entity/animeCollectionsOpenings.entity'
import { AnimeCollectionDTO } from './dto/AnimeCollection.dto'
import { AnimeEntity } from '../anime/entity/anime.entity'
import { OpeningsEntity } from '../opening/entity/opening.entity'

@Injectable()
export class AnimeCollectionsService {
    constructor(
        @InjectRepository(AnimeCollections) private readonly AnimeCollectionsRepository: Repository<AnimeCollections>,
        @InjectRepository(AnimeCollectionOpenings) private readonly AnimeCollectionsOpeningsRepository: Repository<AnimeCollectionOpenings>,
        @InjectEntityManager() private entityManager: EntityManager
    ) {}

    async findAll() {
        const animeCollection = await this.entityManager.query(
            `select *, (select json_agg(row_to_json(openingCollection)) from (select *, (select row_to_json(opening) as opening from (SELECT *,
                (SELECT row_to_json(anime) FROM (select * from "anime" where "openings"."animeId" = "anime".id) as anime) as anime
                FROM public.openings where "animeCollectionOpenings"."animeOpeningId"  = "openings"."id") as opening)
                from "animeCollectionOpenings"
                where "animeCollectionOpenings"."animeCollectionId" = "animeCollections"."id" 
                ) as openingCollection) as openings
                from "animeCollections"`
        )
        return animeCollection
    }
    
    async create(animeCollectionReq : AnimeCollectionDTO) {
        const animeCollection = await this.AnimeCollectionsRepository.save({
            name: animeCollectionReq.name,
            description: animeCollectionReq.description
        })

        for (let i = 0; i < animeCollectionReq.openings.length; i++) {
            const opening = animeCollectionReq.openings[i]
            await this.AnimeCollectionsOpeningsRepository.save({
                animeCollection: animeCollection.id,
                animeOpening: opening
            })
        }

        return animeCollection
    }
}