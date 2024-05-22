import { Controller, Get, Post, Param, Body, Put, Query, Headers, HttpException, Header } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags, ApiOAuth2, ApiBearerAuth } from '@nestjs/swagger'
import { AnimeCollectionsService } from './animeCollections.service'
import { AnimeCollectionDTO } from './dto/AnimeCollection.dto'
import { GetAnimeCollectionsRequest } from './schema/animeCollections.schema'

@ApiTags("Аниме коллекции")
@Controller("animeCollections")
export class AnimeCollectionsController {
  constructor(private readonly service: AnimeCollectionsService) {}

  @Header('Content-Security-Policy', "*localhost*")
  @Get()
  async getAnimeCollections(@Query() query: GetAnimeCollectionsRequest) {
    return await this.service.findAll(query)
  }

  @Header('Content-Security-Policy', "*localhost*")
  @Post()
  @ApiBearerAuth()
  async create(@Body() AnimeCollectionDTO : AnimeCollectionDTO, @Headers() header) {
    if (header["authorization"] == null) {
      throw new HttpException("Пользователь не авторизован", 401)
    }
    
    const token = header["authorization"].split(" ")[1]
    return await this.service.create(AnimeCollectionDTO, token)
  }
}
