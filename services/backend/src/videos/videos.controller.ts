import { Controller, Get, Post, Param, Body, Delete, Res, HttpException, Query, Header } from '@nestjs/common'
import { ApiBody, ApiOperation, ApiResponse, ApiTags } from '@nestjs/swagger'
import { VideosService } from './videos.service'
import { VideoSchemaById404, VideoSchemaById200, VideoSchemaByAnime200, VideoSchemaByAnime404 } from './schema/videos.schema'
import { VideosQueryDTO } from './dto/videos.dto'

@ApiTags("Видео")
@Controller("videos")
export class VideosController {
  constructor(private readonly videosService: VideosService) {}

  @Get("/:id")
  @ApiOperation({ summary: "Получение видео"})
  @ApiResponse({ status: 200, type: VideoSchemaById200})
  @ApiResponse({ status: 404, type: VideoSchemaById404})
  async getVideoById(@Param('id') id: number) {
    return await this.videosService.getVideoById(id)
  }

  @Get("/anime/:id")
  @ApiOperation({ summary: "Получение всех видео у аниме"})
  @ApiResponse({ status: 200, type: VideoSchemaByAnime200, isArray: true})
  @ApiResponse({ status: 404, type: VideoSchemaByAnime404})
  async getVideosByAnime(@Param('id') id: number) {
    return await this.videosService.getVideosByAnime(id)
  }

  @Get()
  @ApiOperation({ summary: "Получение всех видео"})
  async getAllVideo(@Query() query: VideosQueryDTO) {
    return await this.videosService.getAllVideo(query)
  }
}
