import { HttpException, Injectable } from '@nestjs/common';
import { QueryFailedError, Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { AnimeEntity } from './entity/anime.entity';
import { GetAnimeRequest } from './schema/getAnime.schema'

@Injectable()
export class AnimeService {
  constructor(
    @InjectRepository(AnimeEntity) private readonly AnimeRepository: Repository<AnimeEntity>,
  ) { }

  async getAnime(query: GetAnimeRequest): Promise<Object>{
    let genres = []

    if (typeof query.genres == "string") {
        genres.push(query.genres)
    } else {
        genres = [...query.genres]
    }
    
    const querySQLBuilder = this.AnimeRepository.createQueryBuilder("anime")
    querySQLBuilder.innerJoinAndSelect("anime.videos", "videos")
    querySQLBuilder.leftJoinAndSelect("anime.genres", "genresAnime")
    querySQLBuilder.leftJoinAndSelect("genresAnime.genre", "genres")

    querySQLBuilder.where(qb => {
        const subQuery = qb.subQuery()
            .select("anime.id")
            .from("anime", "anime")
            .innerJoin("anime.genres", "genresAnime")
            .leftJoin("genresAnime.genre", "genres")
            .where("genres.id IN (:...genresID)", {genresID: genres})
            .groupBy("anime.id")
            .having("COUNT(genres.id) = :genresCount", {genresCount: genres.length})
            .getQuery()
            
        return "anime.id IN " + subQuery
    })

    querySQLBuilder.skip(query.limit * (query.page - 1))
    querySQLBuilder.take(query.limit)
    const result = await querySQLBuilder.getMany()

    return result;
  }
}
