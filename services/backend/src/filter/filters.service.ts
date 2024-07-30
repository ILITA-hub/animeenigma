import { HttpException, Injectable } from '@nestjs/common';
import { Repository, createQueryBuilder, EntityManager } from 'typeorm';
import { InjectEntityManager, InjectRepository } from '@nestjs/typeorm'
import { CachesService } from '../caches/caches.service'
import { UserEntity } from '../users/entity/user.entity';
import { VideosEntity } from 'src/videos/entity/videos.entity';
import { AnimeEntity } from 'src/anime/entity/anime.entity';
import { GenresAnimeEntity } from 'src/genresAnime/entity/genresAnime.entity';
import { GenresEntity } from 'src/genres/entity/genres.entity';

@Injectable()
export class FiltersService {
    constructor(
        @InjectRepository(AnimeEntity) private readonly AnimeRepository: Repository<AnimeEntity>,
        @InjectRepository(GenresEntity) private readonly GenreReposutory: Repository<GenresEntity>,
    ) { }

    async getYear() {
        let result = []

        const anime = this.AnimeRepository.createQueryBuilder("anime")
            // .groupBy("anime.year")
            .select("anime.year")
            .distinctOn(["anime.year"])

        for (let animeItem of await anime.getMany()) {
            result.push(animeItem.year)
        }

        return result
    }

    async getGenres() {
        return this.GenreReposutory.find({
          where : { active: true },
          select : { id: true, nameRu: true, name: true }
        })
      }
}