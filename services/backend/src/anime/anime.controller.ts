import { Controller, Get, Header, Query } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags, ApiOperation } from '@nestjs/swagger'
import { AnimeService } from './anime.service'
import { GetAnimeRequest, GetAnimeResponse } from './schema/getAnime.schema'

@ApiTags("Аниме")
@Controller("anime")
export class AnimeController {
  constructor(private readonly animeService: AnimeService) {}

  @Get()
  @ApiOperation({ summary: "Получение всех аниме"})
  @ApiResponse({ status: 200, type: GetAnimeResponse, isArray: true})
  async getAnime(@Query() query: GetAnimeRequest) {
    return await this.animeService.getAnime(query)
  }

  @Get("/test")
  @ApiOperation({ summary: "Получение всех аниме"})
  @ApiResponse({ status: 200, type: GetAnimeResponse, isArray: true})
  async getAnime2(@Query() query: GetAnimeRequest) {
    return await this.animeService.getAnimeAll(query)
  }
}
