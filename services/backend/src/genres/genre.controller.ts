import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { GenreService } from './genre.service'

@ApiTags("Жанры")
@Controller("genre")
export class GenreController {
  constructor(private readonly genreService: GenreService) {}

  @Get()
  async getAll() {
    return await this.genreService.getAll()
  }
}
