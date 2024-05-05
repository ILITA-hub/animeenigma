import { HttpException, Injectable } from '@nestjs/common';
import { Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { VideosEntity } from './entity/videos.entity';
import { AnimeEntity } from '../anime/entity/anime.entity'

@Injectable()
export class VideosService {
  constructor(
    @InjectRepository(VideosEntity) private readonly videosReposutory: Repository<VideosEntity>,
    @InjectRepository(AnimeEntity) private readonly animeReposutory: Repository<AnimeEntity>
  ) {}

  async getVideoById(id: number) {
    const result = await this.videosReposutory.findOne({
      where: { id: id, active: true},
      select: { id: true, mp4Path: true, name: true, kind: true}
    })

    if (result == null) {
      throw new HttpException("Видео не найдено", 404)
    }

    return result;
  }

  async getVideosByAnime(id: number) {
    const result = await this.animeReposutory.findOne({
      select: {
        id: true,
        videos: {
          id: true,
          mp4Path: true,
          name: true,
          kind: true
        }
      },
      where: {id: id, active: true},
      relations: { videos: true }
    })
    
    if (result == null) {
      throw new HttpException("Аниме не найдено", 404)
    }

    return result
  }
}
