import { HttpException, Injectable } from '@nestjs/common';
import { Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { VideosEntity } from './entity/videos.entity';
import { AnimeEntity } from '../anime/entity/anime.entity'
import { VideosQueryDTO } from './dto/videos.dto'

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
    const result = await this.videosReposutory.find({
      select: {
        id: true,
        mp4Path: true,
        name: true,
        kind: true
      },
      where: {
        active: true,
        anime: {
          id: id
        }
      }
    })
    
    if (result == null) {
      throw new HttpException("Аниме не найдено", 404)
    }

    return result
  }

  async getAllVideo(query: VideosQueryDTO) {
    const [result, count] = await this.videosReposutory.findAndCount({
      select: {
        id: true,
        mp4Path: true,
        name: true,
        kind: true,
        anime: {
          id: true,
          name: true,
          nameJP: true,
          nameRU: true,
          imgPath: true,
          year: true
        }
      },
      where: {
        active: true
      },
      take: query.limit,
      skip: query.limit * (query.page - 1),
      relations: {
        anime: true
      }
    })

    const allPage = Math.ceil(count/query.limit)
    const prevPage = (query.page <= 1) ? 1 : (query.page > allPage) ? allPage : Number(query.page) - 1
    const nextPage = (query.page >= allPage) ? allPage : Number(query.page) + 1

    return {
      prevPage: prevPage,
      page: Number(query.page),
      nextPage: nextPage,
      allPage: allPage,
      allVideos: count,
      data: result
    }
  } 
}
