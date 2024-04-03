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
    const result = await this.AnimeRepository.createQueryBuilder("anime")
    .innerJoinAndSelect("anime.videos", "videos")
    .innerJoinAndSelect("anime.genres", "genresAnime")
    .innerJoinAndSelect("genresAnime.genre", "genres")
    .skip(query.limit * (query.page - 1))
    .take(query.limit)
    .getMany()

    return result;
  }
}
