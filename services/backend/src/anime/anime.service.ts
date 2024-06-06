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
    console.log(query.year)
    let genres = []
    if (typeof query.genres == "string") {
      genres.push(query.genres)
    } else if (typeof query.genres == "object") {
      genres = [...query.genres]
    }

    const querySQLBuilder = this.AnimeRepository.createQueryBuilder("anime")
    querySQLBuilder.innerJoinAndSelect("anime.videos", "videos")
    querySQLBuilder.leftJoinAndSelect("anime.genres", "genresAnime")
    querySQLBuilder.leftJoinAndSelect("genresAnime.genre", "genres")

    if (genres.length != 0 || query.year != undefined) {
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

        if (query.year != undefined) {
          subQuery.andWhere("anime.year = :year", { year: query.year })
        }

        return "anime.id IN " + subQuery.getQuery()
      })

    }

    const count = await querySQLBuilder.getCount()
    const allPage = Math.ceil(count/query.limit)
    const prevPage = (query.page <= 1) ? 1 : (query.page > allPage) ? allPage : query.page - 1
    const nextPage = (query.page >= allPage) ? allPage : Number(query.page) + 1 // какава хуя оно в строку переделывается АААААААА, теперь будут стоять тут NUMBER

    querySQLBuilder.skip(query.limit * (query.page - 1))
    querySQLBuilder.take(query.limit)
    querySQLBuilder.select(["anime.id", "anime.name", "anime.nameRU", "anime.nameJP", "anime.year", "anime.imgPath", "videos.id", "videos.name", "videos.kind"])
    const resultAnime = await querySQLBuilder.getMany()

    const result = {
      prevPage : prevPage,
      page: Number(query.page),
      nextPage : nextPage,
      allPage : allPage,
      countAnime : count,
      data : resultAnime
    }

    return result;
  }
}
