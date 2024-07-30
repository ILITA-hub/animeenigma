import { Controller, Get, Post, Param, Body, Delete, Res, HttpException, Header } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { GenreService } from './genre.service'

@ApiTags("Жанры")
@Controller("genre")
export class GenreController {
  constructor(private readonly genreService: GenreService) {}  
}
