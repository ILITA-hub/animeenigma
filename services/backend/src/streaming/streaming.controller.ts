import {
  Controller,
  Get,
  Post,
  Query,
  Param,
  Body,
  Res,
  Headers,
  ParseIntPipe,
  UseGuards,
  UseInterceptors,
  UploadedFile,
} from '@nestjs/common';
import { FileInterceptor } from '@nestjs/platform-express';
import { Response } from 'express';
import {
  ApiTags,
  ApiOperation,
  ApiResponse,
  ApiBearerAuth,
  ApiConsumes,
  ApiBody,
} from '@nestjs/swagger';
import { StreamingService } from './streaming.service';
import {
  CreateStreamingSourceDto,
  AddAnimeSourceDto,
  GetStreamingSourcesDto,
  StreamingInfoResponseDto,
  UploadVideoDto,
  ExternalApiSearchDto,
} from './dto/streaming.dto';
import { JwtAuthGuard } from '../auth/guards/jwt.guard';
import { VideoQuality } from './entity/anime-source.entity';

@ApiTags('Streaming')
@Controller('streaming')
export class StreamingController {
  constructor(private readonly streamingService: StreamingService) {}

  // ==================== Streaming Source Management ====================

  @Get('sources')
  @ApiOperation({ summary: 'Get all streaming sources' })
  @ApiResponse({ status: 200, description: 'List of streaming sources' })
  async getAllSources() {
    return this.streamingService.getAllStreamingSources();
  }

  @Post('sources')
  @UseGuards(JwtAuthGuard)
  @ApiBearerAuth()
  @ApiOperation({ summary: 'Create a new streaming source (admin)' })
  @ApiResponse({ status: 201, description: 'Streaming source created' })
  async createSource(@Body() dto: CreateStreamingSourceDto) {
    return this.streamingService.createStreamingSource(dto);
  }

  // ==================== Anime Source Management ====================

  @Get('anime/:animeId')
  @ApiOperation({
    summary: 'Get streaming sources for an anime',
    description: 'Returns all available streaming options for an anime. Frontend can use this to show available sources and players.'
  })
  @ApiResponse({ status: 200, description: 'List of streaming options', type: [StreamingInfoResponseDto] })
  async getAnimeSources(
    @Param('animeId', ParseIntPipe) animeId: number,
    @Query('episode') episode?: number,
    @Query('season') season?: number,
    @Query('translation') translation?: string,
  ): Promise<StreamingInfoResponseDto[]> {
    return this.streamingService.getAnimeSources({
      animeId,
      episode: episode ? Number(episode) : undefined,
      season: season ? Number(season) : undefined,
      translation,
    });
  }

  @Post('anime/source')
  @UseGuards(JwtAuthGuard)
  @ApiBearerAuth()
  @ApiOperation({ summary: 'Add a streaming source for an anime (admin)' })
  @ApiResponse({ status: 201, description: 'Source added' })
  async addAnimeSource(@Body() dto: AddAnimeSourceDto) {
    return this.streamingService.addAnimeSource(dto);
  }

  // ==================== Video Streaming ====================

  @Get('stream/:sourceId')
  @ApiOperation({
    summary: 'Stream video by source ID',
    description: 'Streams video from MinIO or proxies from external source. Supports range requests for seeking.'
  })
  @ApiResponse({ status: 200, description: 'Video stream' })
  @ApiResponse({ status: 206, description: 'Partial content (range request)' })
  @ApiResponse({ status: 404, description: 'Stream not found' })
  async streamVideo(
    @Param('sourceId', ParseIntPipe) sourceId: number,
    @Res() response: Response,
    @Headers('range') range?: string,
  ) {
    return this.streamingService.getStreamById(sourceId, response, range);
  }

  @Get('proxy/:sourceId')
  @ApiOperation({
    summary: 'Proxy stream from external source',
    description: 'Proxies video stream from external API through backend. Use when external source requires authentication or CORS blocking.'
  })
  @ApiResponse({ status: 200, description: 'Proxied video stream' })
  async proxyStream(
    @Param('sourceId', ParseIntPipe) sourceId: number,
    @Res() response: Response,
    @Headers('range') range?: string,
  ) {
    return this.streamingService.proxyStream(sourceId, response, range);
  }

  // ==================== MinIO Upload ====================

  @Post('upload')
  @UseGuards(JwtAuthGuard)
  @ApiBearerAuth()
  @UseInterceptors(FileInterceptor('video'))
  @ApiConsumes('multipart/form-data')
  @ApiOperation({
    summary: 'Upload video to MinIO (admin)',
    description: 'Upload a video file to MinIO storage. For self-hosted content.'
  })
  @ApiBody({
    schema: {
      type: 'object',
      properties: {
        video: { type: 'string', format: 'binary' },
        animeId: { type: 'number' },
        episode: { type: 'number' },
        season: { type: 'number' },
        translation: { type: 'string' },
        quality: { type: 'string', enum: Object.values(VideoQuality) },
      },
      required: ['video', 'animeId'],
    },
  })
  @ApiResponse({ status: 201, description: 'Video uploaded successfully' })
  async uploadVideo(
    @UploadedFile() file: Express.Multer.File,
    @Body('animeId', ParseIntPipe) animeId: number,
    @Body('episode') episode?: string,
    @Body('season') season?: string,
    @Body('translation') translation?: string,
    @Body('quality') quality?: VideoQuality,
  ) {
    return this.streamingService.uploadToMinio(
      animeId,
      file,
      episode ? parseInt(episode, 10) : undefined,
      season ? parseInt(season, 10) : undefined,
      translation,
      quality,
    );
  }

  // ==================== External API Integration ====================

  @Get('external/search')
  @ApiOperation({
    summary: 'Search external streaming APIs',
    description: 'Search configured external APIs (Kodik, Anilibria, etc.) for anime. Returns embed URLs for frontend direct use or stream URLs for backend proxy.'
  })
  @ApiResponse({ status: 200, description: 'Search results from external APIs' })
  async searchExternalSources(@Query() dto: ExternalApiSearchDto) {
    return this.streamingService.searchExternalSources(dto);
  }

  @Post('external/import')
  @UseGuards(JwtAuthGuard)
  @ApiBearerAuth()
  @ApiOperation({
    summary: 'Import external source to local DB (admin)',
    description: 'Save an external streaming source to local database for persistent access.'
  })
  @ApiResponse({ status: 201, description: 'Source imported' })
  async importExternalSource(
    @Body('animeId', ParseIntPipe) animeId: number,
    @Body('sourceType') sourceType: string,
    @Body('externalId') externalId: string,
    @Body('translation') translation: string,
    @Body('embedUrl') embedUrl?: string,
    @Body('directUrl') directUrl?: string,
  ) {
    return this.streamingService.importExternalSource(
      animeId,
      sourceType,
      externalId,
      translation,
      embedUrl,
      directUrl,
    );
  }

  // ==================== Info Endpoint ====================

  @Get('info')
  @ApiOperation({
    summary: 'Get streaming service info',
    description: 'Returns information about available streaming sources and capabilities.'
  })
  @ApiResponse({ status: 200, description: 'Streaming service info' })
  async getInfo() {
    const sources = await this.streamingService.getAllStreamingSources();
    return {
      availableSources: sources.map(s => ({
        name: s.name,
        displayName: s.displayName,
        type: s.type,
        requiresProxy: s.requiresProxy,
      })),
      capabilities: {
        minioUpload: true,
        externalSearch: true,
        proxyStreaming: true,
        embedSupport: true,
      },
      config: {
        supportedQualities: Object.values(VideoQuality),
        maxUploadSize: '500MB',
      },
    };
  }
}
