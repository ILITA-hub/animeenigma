import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { AnimeCollectionsService } from './animeCollections.service'
import { AnimeCollectionDTO } from './dto/AnimeCollection.dto'

@ApiTags("AnimeCollection")
@Controller("animeCollections")
export class AnimeCollectionsController {
  constructor(private readonly service: AnimeCollectionsService) {}

  @Get("getAll")
  async getAll() {
    return await this.service.findAll()
  }

  @Post()
  async create(@Body() AnimeCollectionDTO : AnimeCollectionDTO) {
    return await this.service.create(AnimeCollectionDTO)
  }
}
