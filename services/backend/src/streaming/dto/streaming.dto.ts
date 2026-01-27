import { ApiProperty, ApiPropertyOptional } from '@nestjs/swagger';
import { IsString, IsOptional, IsNumber, IsEnum, IsBoolean, IsUrl, Min, Max } from 'class-validator';
import { Type } from 'class-transformer';
import { StreamingSourceType } from '../entity/streaming-source.entity';
import { VideoQuality } from '../entity/anime-source.entity';

export class CreateStreamingSourceDto {
  @ApiProperty({ description: 'Unique name identifier (e.g., "kodik", "minio")' })
  @IsString()
  name: string;

  @ApiPropertyOptional({ description: 'Display name for UI' })
  @IsOptional()
  @IsString()
  displayName?: string;

  @ApiProperty({ description: 'Source type', enum: StreamingSourceType })
  @IsEnum(StreamingSourceType)
  type: StreamingSourceType;

  @ApiPropertyOptional({ description: 'Base URL for API' })
  @IsOptional()
  @IsString()
  baseUrl?: string;

  @ApiPropertyOptional({ description: 'API key if required' })
  @IsOptional()
  @IsString()
  apiKey?: string;

  @ApiPropertyOptional({ description: 'Whether backend needs to proxy the stream' })
  @IsOptional()
  @IsBoolean()
  requiresProxy?: boolean;

  @ApiPropertyOptional({ description: 'Priority (higher = preferred)' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  priority?: number;

  @ApiPropertyOptional({ description: 'Additional configuration' })
  @IsOptional()
  config?: Record<string, any>;
}

export class AddAnimeSourceDto {
  @ApiProperty({ description: 'Local anime ID' })
  @Type(() => Number)
  @IsNumber()
  animeId: number;

  @ApiProperty({ description: 'Streaming source ID' })
  @Type(() => Number)
  @IsNumber()
  streamingSourceId: number;

  @ApiPropertyOptional({ description: 'External ID in the source system' })
  @IsOptional()
  @IsString()
  externalId?: string;

  @ApiPropertyOptional({ description: 'Episode number' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  episode?: number;

  @ApiPropertyOptional({ description: 'Season number' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  season?: number;

  @ApiPropertyOptional({ description: 'Translation/dub name' })
  @IsOptional()
  @IsString()
  translation?: string;

  @ApiPropertyOptional({ description: 'Translation type (voice/subtitles)' })
  @IsOptional()
  @IsString()
  translationType?: string;

  @ApiPropertyOptional({ description: 'Direct stream URL' })
  @IsOptional()
  @IsString()
  directUrl?: string;

  @ApiPropertyOptional({ description: 'Embed player URL' })
  @IsOptional()
  @IsString()
  embedUrl?: string;

  @ApiPropertyOptional({ description: 'Path in MinIO bucket' })
  @IsOptional()
  @IsString()
  minioPath?: string;

  @ApiPropertyOptional({ description: 'Video quality', enum: VideoQuality })
  @IsOptional()
  @IsEnum(VideoQuality)
  quality?: VideoQuality;

  @ApiPropertyOptional({ description: 'Available quality options' })
  @IsOptional()
  availableQualities?: string[];

  @ApiPropertyOptional({ description: 'Additional metadata' })
  @IsOptional()
  metadata?: Record<string, any>;
}

export class GetStreamingSourcesDto {
  @ApiProperty({ description: 'Anime ID to get sources for' })
  @Type(() => Number)
  @IsNumber()
  animeId: number;

  @ApiPropertyOptional({ description: 'Episode number' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  episode?: number;

  @ApiPropertyOptional({ description: 'Season number' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  season?: number;

  @ApiPropertyOptional({ description: 'Filter by translation' })
  @IsOptional()
  @IsString()
  translation?: string;
}

export class StreamingInfoResponseDto {
  @ApiProperty({ description: 'Source type' })
  type: 'minio' | 'proxy' | 'direct' | 'embed';

  @ApiPropertyOptional({ description: 'Direct URL (for direct and minio types)' })
  url?: string;

  @ApiPropertyOptional({ description: 'Embed URL (for embed type)' })
  embedUrl?: string;

  @ApiPropertyOptional({ description: 'Proxy endpoint (for proxy type)' })
  proxyEndpoint?: string;

  @ApiProperty({ description: 'Translation/dub info' })
  translation?: string;

  @ApiProperty({ description: 'Available qualities' })
  availableQualities?: string[];

  @ApiProperty({ description: 'Source name' })
  sourceName: string;
}

export class UploadVideoDto {
  @ApiProperty({ description: 'Local anime ID' })
  @Type(() => Number)
  @IsNumber()
  animeId: number;

  @ApiPropertyOptional({ description: 'Episode number' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  episode?: number;

  @ApiPropertyOptional({ description: 'Season number' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  season?: number;

  @ApiPropertyOptional({ description: 'Translation name' })
  @IsOptional()
  @IsString()
  translation?: string;

  @ApiPropertyOptional({ description: 'Video quality', enum: VideoQuality })
  @IsOptional()
  @IsEnum(VideoQuality)
  quality?: VideoQuality;
}

export class ExternalApiSearchDto {
  @ApiProperty({ description: 'Anime name to search' })
  @IsString()
  query: string;

  @ApiPropertyOptional({ description: 'Shikimori ID for better matching' })
  @IsOptional()
  @Type(() => Number)
  @IsNumber()
  shikimoriId?: number;
}
