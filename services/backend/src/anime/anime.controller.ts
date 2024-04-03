import { Controller, Get, Post, Param, Body, Delete, Res, HttpException, HttpCode, Query } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags, ApiOperation } from '@nestjs/swagger'
import { AnimeService } from './anime.service'
import * as bcrypt from 'bcrypt'
import { GetAnimeRequest, GetAnimeResponse } from './schema/getAnime.schema'

@ApiTags("Аниме")
@Controller("anime")
export class AnimeController {
  constructor(private readonly animeService: AnimeService) {}

  @Get()
  @ApiOperation({ summary: "Получение всех комнат"})
  @ApiResponse({ status: 200, type: GetAnimeResponse, isArray: true})
  async getAnime(@Query() query: GetAnimeRequest) {
    return await this.animeService.getAnime(query)
  }
}
