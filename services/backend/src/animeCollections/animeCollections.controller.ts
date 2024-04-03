import { Controller, Get, Post, Param, Body, Put, Query } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { AnimeCollectionsService } from './animeCollections.service'
import { AnimeCollectionDTO } from './dto/AnimeCollection.dto'
import { GetAnimeCollectionsRequest } from './schema/animeCollections.schema'

@ApiTags("Аниме коллекции")
@Controller("animeCollections")
export class AnimeCollectionsController {
  constructor(private readonly service: AnimeCollectionsService) {}

  @Get()
  async getAnimeCollections(@Query() query: GetAnimeCollectionsRequest) {
    return await this.service.findAll(query)
  }

  @Post()
  async create(@Body() AnimeCollectionDTO : AnimeCollectionDTO) {
    return await this.service.create(AnimeCollectionDTO)
  }
}
