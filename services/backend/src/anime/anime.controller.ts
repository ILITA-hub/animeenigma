import { Controller, Get, Post, Param, Body, Delete, Res, HttpException, HttpCode, Query } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { AnimeService } from './anime.service'
import * as bcrypt from 'bcrypt'
import { GetAnimeRequest } from './schema/getAnime.schema'

@ApiTags("Anime")
@Controller("anime")
export class AnimeController {
  constructor(private readonly animeService: AnimeService) {}

  @Get()
  async getAnime(@Query() query: GetAnimeRequest) {
    return await this.animeService.getAnime(query)
  }
}
