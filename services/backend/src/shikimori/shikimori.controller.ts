import { Controller, Get, Post, Query, Param, Body, ParseIntPipe, UseGuards } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse, ApiBearerAuth } from '@nestjs/swagger';
import { ShikimoriService } from './shikimori.service';
import { SearchAnimeDto, ShikimoriAnimeResponseDto, MapAnimeDto } from './dto/shikimori.dto';
import { JwtAuthGuard } from '../auth/guards/jwt.guard';

@ApiTags('Shikimori')
@Controller('shikimori')
export class ShikimoriController {
  constructor(private readonly shikimoriService: ShikimoriService) {}

  @Get('search')
  @ApiOperation({
    summary: 'Search anime on Shikimori',
    description: 'Search anime by name. Results are cached locally for faster subsequent access.'
  })
  @ApiResponse({ status: 200, description: 'Search results', type: [ShikimoriAnimeResponseDto] })
  async searchAnime(@Query() dto: SearchAnimeDto): Promise<ShikimoriAnimeResponseDto[]> {
    return this.shikimoriService.searchAnime(dto);
  }

  @Get('anime/:shikimoriId')
  @ApiOperation({
    summary: 'Get anime details by Shikimori ID',
    description: 'Get detailed anime information. Fetches from API if not cached or outdated.'
  })
  @ApiResponse({ status: 200, description: 'Anime details' })
  @ApiResponse({ status: 404, description: 'Anime not found' })
  async getAnimeById(@Param('shikimoriId', ParseIntPipe) shikimoriId: number) {
    return this.shikimoriService.getAnimeById(shikimoriId);
  }

  @Post('map')
  @UseGuards(JwtAuthGuard)
  @ApiBearerAuth()
  @ApiOperation({
    summary: 'Map Shikimori anime to local database',
    description: 'Maps a Shikimori anime to the local anime database. Creates a new local entry if localAnimeId is not provided.'
  })
  @ApiResponse({ status: 200, description: 'Anime mapped successfully' })
  @ApiResponse({ status: 404, description: 'Anime not found' })
  async mapAnime(@Body() dto: MapAnimeDto) {
    return this.shikimoriService.mapToLocalAnime(dto.shikimoriId, dto.localAnimeId);
  }

  @Get('mapped')
  @ApiOperation({
    summary: 'Get all mapped anime',
    description: 'Get list of all anime that have been mapped from Shikimori to local database.'
  })
  @ApiResponse({ status: 200, description: 'List of mapped anime' })
  async getMappedAnime(
    @Query('page', new ParseIntPipe({ optional: true })) page = 1,
    @Query('limit', new ParseIntPipe({ optional: true })) limit = 20,
  ) {
    return this.shikimoriService.getMappedAnime(page, limit);
  }
}
