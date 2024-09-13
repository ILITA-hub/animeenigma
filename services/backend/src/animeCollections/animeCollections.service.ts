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
import { AnimeEntity } from 'src/anime/entity/anime.entity';
import { GenresAnimeEntity } from 'src/genresAnime/entity/genresAnime.entity';

@Injectable()
export class AnimeCollectionsService {
    constructor(
        @InjectRepository(AnimeCollections) private readonly AnimeCollectionsRepository: Repository<AnimeCollections>,
        @InjectRepository(AnimeCollectionOpenings) private readonly AnimeCollectionsOpeningsRepository: Repository<AnimeCollectionOpenings>,
        @InjectRepository(UserEntity) private readonly UserRepository: Repository<UserEntity>,
        @InjectRepository(VideosEntity) private readonly VideosRepository: Repository<VideosEntity>,
        @InjectRepository(GenresAnimeEntity) private readonly GenresAnimeRepository: Repository<GenresAnimeEntity>,
        @InjectRepository(AnimeEntity) private readonly AnimeRepository: Repository<AnimeEntity>,
        private readonly cachesService: CachesService,
    ) { }

    async findAll(query: GetAnimeCollectionsRequest) {
        let genres = []
        let years = []

        if (query.genres) genres = typeof query.genres == "object" ? [...query.genres] : [query.genres]
        if (query.year) years = typeof query.year == "object" ? [...query.year] : [query.year]

        const videos = await this.getVideosIds(years, genres)

        const querySQLBuilder = this.AnimeCollectionsRepository.createQueryBuilder("animeCollections")

        if (genres.length != 0 || years.length != 0) {
            querySQLBuilder.innerJoinAndSelect("animeCollections.openings", "animeCollectionOpenings")
            querySQLBuilder.leftJoinAndSelect("animeCollectionOpenings.animeOpening", "videos")
            querySQLBuilder.andWhere("videos.id IN (:...videos)", { videos: videos })
        }

        querySQLBuilder.select(["animeCollections.id", "animeCollections.name"])

        const count = await querySQLBuilder.getCount()
        const allPage = Math.ceil(count / query.limit)
        const prevPage = (query.page <= 1) ? 1 : (query.page > allPage) ? allPage : query.page - 1
        const nextPage = (query.page >= allPage) ? allPage : Number(query.page) + 1 // какава хуя оно в строку переделывается АААААААА, теперь будут стоять тут NUMBER

        const resultCollections = await querySQLBuilder.getMany()
        let resultColl = []

        for (let collection of resultCollections) {
            let coll = {
                id: collection.id,
                name: collection.name,
                genres: []
            }

            const animeRequest = this.AnimeCollectionsRepository.createQueryBuilder("animeCollections").andWhere(`animeCollections.id = ${collection.id}`)
            animeRequest.innerJoinAndSelect("animeCollections.openings", "animeCollectionOpenings")
            animeRequest.leftJoinAndSelect("animeCollectionOpenings.animeOpening", "videos")
            animeRequest.leftJoinAndSelect("videos.anime", "anime")

            let videosArray = await animeRequest.getMany(); 
            for (let openign of videosArray) {
                let animeId = openign.openings[0].animeOpening.anime.id

                let genresCol = this.GenresAnimeRepository.createQueryBuilder("genresAnime")
                    .andWhere(`genresAnime.animeId = ${animeId}`)
                    .innerJoinAndSelect("genresAnime.genre", "genre")

                for (let genre of await genresCol.getMany()) {
                    coll.genres.push(genre.genre["nameRu"])
                }
            }

            resultColl.push(coll)
        }

        const result = {
            prevPage: prevPage,
            page: Number(query.page),
            nextPage: nextPage,
            allPage: allPage,
            countAnime: count,
            data: resultColl
        }

        return result
    }

    private async getVideosIds(year, genre) {

        let genres = []
        let years = []

        if (genre) genres = typeof genre == "object" ? [...genre] : [genre]
        if (year) years = typeof year == "object" ? [...year] : [year]

        const querySQLBuilder = this.AnimeRepository.createQueryBuilder("anime")
        querySQLBuilder.innerJoinAndSelect("anime.videos", "videos")
        querySQLBuilder.leftJoinAndSelect("anime.genres", "genresAnime")
        querySQLBuilder.leftJoinAndSelect("genresAnime.genre", "genres")
        querySQLBuilder.andWhere("anime.active = true")

        if (genres.length != 0) {
            querySQLBuilder.where(qb => {
                const subQuery = qb.subQuery()
                subQuery.select("anime.id")
                subQuery.from("anime", "anime")
                subQuery.where("anime.active = true")
                subQuery.innerJoin("anime.genres", "genresAnime")
                subQuery.leftJoin("genresAnime.genre", "genres")
                if (genres.length != 0) {
                    subQuery.andWhere("genres.id IN (:...genresID)", { genresID: genres })
                    subQuery.groupBy("anime.id")
                    subQuery.having("COUNT(genres.id) = :genresCount", { genresCount: genres.length })
                }

                return "anime.id IN " + subQuery.getQuery()
            })
        }

        if (years.length != 0) {
            querySQLBuilder.andWhere("anime.year IN (:...years)", { years: years })
        }

        querySQLBuilder.select(["anime.id", "videos.id"])

        const resultAnime = await querySQLBuilder.getMany()

        let videos = []

        resultAnime.forEach(anime => {
            anime.videos.forEach(video => {
                videos.push(video.id)
            })
        })

        return videos;
    }

    async create(animeCollectionReq: AnimeCollectionDTO, token: string) {
        const session = await this.cachesService.getCache(`userSession${token}`)

        if (session == null) {
            throw new HttpException("Пользователь не авторизован", 401)
        }

        const collections = await this.AnimeCollectionsRepository.save({
            name: animeCollectionReq.name,
            description: animeCollectionReq.description,
            owner: await this.UserRepository.findOneBy({ id: session["userId"] })
        })

        for (let i = 0; i < animeCollectionReq.openings.length; i++) {
            const opening = await this.VideosRepository.findOneBy({ id:  animeCollectionReq.openings[i] })
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

    async getInfoById(id: number) {
        const query = this.AnimeCollectionsRepository.createQueryBuilder("animeCollections")
        query.andWhere("animeCollections.id = :id", {id: id})
        query.innerJoinAndSelect("animeCollections.openings", "animeCollectionOpenings")
        query.leftJoinAndSelect("animeCollectionOpenings.animeOpening", "videos")

        return await query.getOne();
    }
}