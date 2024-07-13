import { HttpException, Injectable } from '@nestjs/common';
import { QueryFailedError, Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { AnimeEntity } from './entity/anime.entity';
import { GetAnimeRequest } from './schema/getAnime.schema'
import { count } from 'console';

@Injectable()
export class AnimeService {
  constructor(
    @InjectRepository(AnimeEntity) private readonly AnimeRepository: Repository<AnimeEntity>,
  ) { }

  async getAnime(query: GetAnimeRequest) {
    let genres = []
    let years = []

    if (query.genres) genres = typeof query.genres == "object" ? [...query.genres] : [query.genres]
    if (query.year) years = typeof query.year == "object" ? [...query.year] : [query.year]

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
      querySQLBuilder.andWhere("anime.year IN (:...years)", {years: years})
    }

    querySQLBuilder.skip(query.limit * (query.page - 1))
    querySQLBuilder.take(query.limit)
    querySQLBuilder.select(["anime.id", "anime.nameRU", "anime.year", "anime.imgPath", "videos.id", "videos.name", "genresAnime.id", "genres.id", "genres.nameRu"])

    const count = await querySQLBuilder.getCount()
    const allPage = Math.ceil(count / query.limit)
    const prevPage = (query.page <= 1) ? 1 : (query.page > allPage) ? allPage : query.page - 1
    const nextPage = (query.page >= allPage) ? allPage : Number(query.page) + 1 // какава хуя оно в строку переделывается АААААААА, теперь будут стоять тут NUMBER

    const resultAnime = await querySQLBuilder.getMany()

    const result = {
      prevPage: prevPage,
      page: Number(query.page),
      nextPage: nextPage,
      allPage: allPage,
      countAnime: count,
      data: resultAnime
    }

    return result;
  }
}
