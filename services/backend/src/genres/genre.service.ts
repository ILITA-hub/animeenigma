import { Injectable } from '@nestjs/common';
import { CachesService } from '../caches/caches.service'
import { Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { GenresEntity } from './entity/genres.entity';
import { where } from 'sequelize';

@Injectable()
export class GenreService {
  constructor(@InjectRepository(GenresEntity) private readonly GenreReposutory: Repository<GenresEntity>) {}

  async getAll() {
    return this.GenreReposutory.find({
      where : { active: true },
      select : { id: true, nameRu: true, name: true }
    })
  }
}
