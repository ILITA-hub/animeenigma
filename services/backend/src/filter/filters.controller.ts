import { Controller, Get, Post, Param, Body, Put, Query, Headers, HttpException, Header } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags, ApiOAuth2, ApiBearerAuth } from '@nestjs/swagger'
import { FiltersService } from './filters.service'

@ApiTags("Для фильтров")
@Controller("filters")
export class FiltersController {
  constructor(private readonly service: FiltersService) {}

  @Get("years")
  async getYear() {
    return await this.service.getYear()
  }

  @Get("genres")
  async getGenre() {
    return await this.service.getGenres()
  }
}
