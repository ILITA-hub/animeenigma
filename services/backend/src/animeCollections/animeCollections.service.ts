import { HttpException, Injectable } from '@nestjs/common';
import { Repository, createQueryBuilder, EntityManager } from 'typeorm';
import { InjectEntityManager, InjectRepository } from '@nestjs/typeorm'
import { AnimeCollections } from './entity/animeCollection.entity'
import { AnimeCollectionOpenings } from './entity/animeCollectionsOpenings.entity'
import { AnimeCollectionDTO } from './dto/AnimeCollection.dto'
import { GetAnimeCollectionsRequest } from './schema/animeCollections.schema'
import { CachesService } from '../caches/caches.service'
import { UserEntity } from '../users/entity/user.entity';
import { VideosEntity } from 'src/videos/entity/videos.entity';

@Injectable()
export class AnimeCollectionsService {
    constructor(
        @InjectRepository(AnimeCollections) private readonly AnimeCollectionsRepository: Repository<AnimeCollections>,
        @InjectRepository(AnimeCollectionOpenings) private readonly AnimeCollectionsOpeningsRepository: Repository<AnimeCollectionOpenings>,
        @InjectRepository(UserEntity) private readonly UserRepository: Repository<UserEntity>,
        @InjectRepository(VideosEntity) private readonly VideosRepository: Repository<VideosEntity>,
        private readonly cachesService: CachesService,
    ) { }

    async findAll(query: GetAnimeCollectionsRequest) {

        let genres = []
        if (typeof query.genres == "string") {
            genres.push(query.genres)
        } else if (typeof query.genres == "object") {
            genres = [...query.genres]
        }

        const querySQLBuilder = this.AnimeCollectionsRepository.createQueryBuilder("animeCollections")
        querySQLBuilder.innerJoinAndSelect("animeCollections.openings", "animeCollectionOpenings")
        querySQLBuilder.leftJoinAndSelect("animeCollectionOpenings.animeOpening", "videos")
        querySQLBuilder.leftJoinAndSelect("videos.anime", "anime")
        querySQLBuilder.leftJoinAndSelect("anime.genres", "genresAnime")
        querySQLBuilder.leftJoinAndSelect("genresAnime.genre", "genres")

        if (genres.length != 0 || query.year != undefined) {

            const subQuery2 = this.AnimeCollectionsRepository.createQueryBuilder()
            subQuery2.select("videos.id")
            subQuery2.from("videos", "videos")
            subQuery2.innerJoin("videos.anime", "anime")
            subQuery2.where(qb2 => {
                const subQuery = qb2.subQuery()
                subQuery.select("anime.id")
                subQuery.from("anime", "anime")
                subQuery.innerJoin("anime.genres", "genresAnime")
                subQuery.leftJoin("genresAnime.genre", "genres")
                if (genres.length != 0) {
                    subQuery.andWhere("genres.id IN (:...genresID)", { genresID: genres })
                    subQuery.having("COUNT(genres.id) = :genresCount", { genresCount: genres.length })
                }

                if (query.year != undefined) {
                    subQuery.andWhere("anime.year = :year", { year: query.year })
                }

                subQuery.groupBy("anime.id")

                return "anime.id IN " + subQuery.getQuery()
            })

            const resultVideos = await subQuery2.getMany()
            if (resultVideos.length == 0) {
                return []
            }
            let arrayVideo = []

            resultVideos.forEach(el => {
                arrayVideo.push(el.id)
            })

            const resultSub2 = "videos.id IN (" + arrayVideo.join(",") + ")"
            const videoCount = (await subQuery2.getMany()).length
            console.log(videoCount)

            querySQLBuilder.where(qb => {
                const subQuery3 = qb.subQuery()
                subQuery3.select("animeCollectionOpenings.id")
                subQuery3.from("animeCollectionOpenings", "animeCollectionOpenings")
                subQuery3.innerJoin("animeCollectionOpenings.animeOpening", "videos")
                subQuery3.where(resultSub2)
                subQuery3.groupBy("animeCollectionOpenings.id")

                const resultSub3 = "animeCollectionOpenings.id IN " + subQuery3.getQuery()

                const subQuery4 = qb.subQuery()
                subQuery4.select("animeCollections.id")
                subQuery4.from("animeCollections", "animeCollections")
                subQuery4.innerJoin("animeCollections.openings", "animeCollectionOpenings")
                subQuery4.where(resultSub3)
                subQuery4.groupBy("animeCollections.id")

                return "animeCollections.id IN " + subQuery4.getQuery()
            })

        }

        querySQLBuilder.select(["animeCollections.id", "animeCollections.name"])
        const result = await querySQLBuilder.getMany()
        return result
    }

    async create(animeCollectionReq: AnimeCollectionDTO, token: string) {
        const session = await this.cachesService.getCache(`userSession${token}`)

        if (session == null) {
            throw new HttpException("Пользователь не авторизован", 401)
        }

        const collections = await this.AnimeCollectionsRepository.save({
            name: animeCollectionReq.name,
            description: animeCollectionReq.description,
            owner: await this.UserRepository.findOneBy({ id: session["userId"]})
        })

        for (let i = 0; i < animeCollectionReq.openings.length; i++) {
            const opening = await this.VideosRepository.findOneBy({ id: animeCollectionReq.openings[i] })
            await this.AnimeCollectionsOpeningsRepository.save({
                animeCollection: collections,
                animeOpening: opening
            })
        }

        return {
            id: collections.id,
            name: collections.name,
            description: collections.description
        }
    }
}