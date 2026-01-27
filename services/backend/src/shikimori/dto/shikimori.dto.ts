import { ApiProperty, ApiPropertyOptional } from '@nestjs/swagger';
import { IsString, IsOptional, IsNumber, IsEnum, Min, Max } from 'class-validator';
import { Type } from 'class-transformer';
import { ShikimoriAnimeKind, ShikimoriAnimeStatus } from '../entity/shikimori-anime.entity';

export class SearchAnimeDto {
  @ApiProperty({ description: 'Search query (anime name)' })
  @IsString()
  query: string;

  @ApiPropertyOptional({ description: 'Page number', default: 1 })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  @Min(1)
  page?: number = 1;

  @ApiPropertyOptional({ description: 'Results per page', default: 20 })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  @Min(1)
  @Max(50)
  limit?: number = 20;

  @ApiPropertyOptional({ description: 'Filter by kind', enum: ShikimoriAnimeKind })
  @IsOptional()
  @IsEnum(ShikimoriAnimeKind)
  kind?: ShikimoriAnimeKind;

  @ApiPropertyOptional({ description: 'Filter by status', enum: ShikimoriAnimeStatus })
  @IsOptional()
  @IsEnum(ShikimoriAnimeStatus)
  status?: ShikimoriAnimeStatus;

  @ApiPropertyOptional({ description: 'Filter by year' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  year?: number;
}

export class ShikimoriAnimeResponseDto {
  @ApiProperty()
  id: number;

  @ApiProperty()
  shikimoriId: number;

  @ApiProperty()
  name: string;

  @ApiPropertyOptional()
  nameRussian?: string;

  @ApiPropertyOptional()
  nameEnglish?: string;

  @ApiPropertyOptional()
  nameJapanese?: string;

  @ApiPropertyOptional()
  kind?: string;

  @ApiPropertyOptional()
  status?: string;

  @ApiPropertyOptional()
  episodes?: number;

  @ApiPropertyOptional()
  episodesAired?: number;

  @ApiPropertyOptional()
  score?: number;

  @ApiPropertyOptional()
  posterUrl?: string;

  @ApiPropertyOptional()
  description?: string;

  @ApiPropertyOptional()
  genres?: Array<{ id: number; name: string; russian: string }>;

  @ApiProperty()
  isMapped: boolean;

  @ApiPropertyOptional()
  localAnimeId?: number;
}

export class MapAnimeDto {
  @ApiProperty({ description: 'Shikimori anime ID' })
  @Type(() => Number)
  @IsNumber()
  shikimoriId: number;

  @ApiPropertyOptional({ description: 'Local anime ID to map to (creates new if not provided)' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  localAnimeId?: number;
}
