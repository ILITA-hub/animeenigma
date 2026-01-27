import { Injectable, Logger, HttpException, HttpStatus, StreamableFile } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { Response } from 'express';
import * as Minio from 'minio';
import axios, { AxiosInstance } from 'axios';
import { Readable } from 'stream';
import { config } from '../config';
import { StreamingSource, StreamingSourceType } from './entity/streaming-source.entity';
import { AnimeSource, VideoQuality } from './entity/anime-source.entity';
import { AnimeEntity } from '../anime/entity/anime.entity';
import {
  CreateStreamingSourceDto,
  AddAnimeSourceDto,
  GetStreamingSourcesDto,
  StreamingInfoResponseDto,
  ExternalApiSearchDto,
} from './dto/streaming.dto';

@Injectable()
export class StreamingService {
  private readonly logger = new Logger(StreamingService.name);
  private readonly minioClient: Minio.Client;
  private readonly externalApis: Map<string, AxiosInstance> = new Map();

  constructor(
    @InjectRepository(StreamingSource)
    private readonly streamingSourceRepo: Repository<StreamingSource>,
    @InjectRepository(AnimeSource)
    private readonly animeSourceRepo: Repository<AnimeSource>,
    @InjectRepository(AnimeEntity)
    private readonly animeRepo: Repository<AnimeEntity>,
  ) {
    // Initialize MinIO client
    this.minioClient = new Minio.Client({
      endPoint: config.minio.endPoint,
      port: config.minio.port,
      useSSL: config.minio.useSSL,
      accessKey: config.minio.accessKey,
      secretKey: config.minio.secretKey,
    });

    // Initialize external API clients
    this.initializeExternalApis();
  }

  /**
   * Initialize HTTP clients for configured external APIs
   */
  private initializeExternalApis(): void {
    const apis = config.externalApis;

    if (apis.kodik?.enabled) {
      this.externalApis.set('kodik', axios.create({
        baseURL: apis.kodik.baseUrl,
        timeout: config.streaming.externalTimeout,
      }));
    }

    if (apis.anilibria?.enabled) {
      this.externalApis.set('anilibria', axios.create({
        baseURL: apis.anilibria.baseUrl,
        timeout: config.streaming.externalTimeout,
      }));
    }
  }

  // ==================== Streaming Source Management ====================

  /**
   * Create a new streaming source
   */
  async createStreamingSource(dto: CreateStreamingSourceDto): Promise<StreamingSource> {
    const existing = await this.streamingSourceRepo.findOne({
      where: { name: dto.name },
    });

    if (existing) {
      throw new HttpException('Streaming source with this name already exists', HttpStatus.CONFLICT);
    }

    const source = this.streamingSourceRepo.create({
      ...dto,
      active: true,
    });

    return this.streamingSourceRepo.save(source);
  }

  /**
   * Get all streaming sources
   */
  async getAllStreamingSources(): Promise<StreamingSource[]> {
    return this.streamingSourceRepo.find({
      where: { active: true },
      order: { priority: 'DESC' },
    });
  }

  /**
   * Initialize default streaming sources (MinIO)
   */
  async initializeDefaultSources(): Promise<void> {
    const minioSource = await this.streamingSourceRepo.findOne({
      where: { name: 'minio' },
    });

    if (!minioSource) {
      await this.createStreamingSource({
        name: 'minio',
        displayName: 'Local Storage (MinIO)',
        type: StreamingSourceType.MINIO,
        baseUrl: config.minio.publicUrl,
        requiresProxy: false,
        priority: 100,
      });
      this.logger.log('Initialized default MinIO streaming source');
    }
  }

  // ==================== Anime Source Management ====================

  /**
   * Add a video source for an anime
   */
  async addAnimeSource(dto: AddAnimeSourceDto): Promise<AnimeSource> {
    const anime = await this.animeRepo.findOne({ where: { id: dto.animeId } });
    if (!anime) {
      throw new HttpException('Anime not found', HttpStatus.NOT_FOUND);
    }

    const streamingSource = await this.streamingSourceRepo.findOne({
      where: { id: dto.streamingSourceId },
    });
    if (!streamingSource) {
      throw new HttpException('Streaming source not found', HttpStatus.NOT_FOUND);
    }

    const source = this.animeSourceRepo.create({
      ...dto,
      active: true,
    });

    return this.animeSourceRepo.save(source);
  }

  /**
   * Get all video sources for an anime
   */
  async getAnimeSources(dto: GetStreamingSourcesDto): Promise<StreamingInfoResponseDto[]> {
    const query = this.animeSourceRepo.createQueryBuilder('source')
      .leftJoinAndSelect('source.streamingSource', 'streamingSource')
      .where('source.animeId = :animeId', { animeId: dto.animeId })
      .andWhere('source.active = true')
      .andWhere('streamingSource.active = true');

    if (dto.episode !== undefined) {
      query.andWhere('(source.episode = :episode OR source.episode IS NULL)', { episode: dto.episode });
    }

    if (dto.season !== undefined) {
      query.andWhere('(source.season = :season OR source.season IS NULL)', { season: dto.season });
    }

    if (dto.translation) {
      query.andWhere('source.translation = :translation', { translation: dto.translation });
    }

    query.orderBy('streamingSource.priority', 'DESC');

    const sources = await query.getMany();

    return sources.map(source => this.formatStreamingInfo(source));
  }

  /**
   * Format anime source to streaming info response
   */
  private formatStreamingInfo(source: AnimeSource): StreamingInfoResponseDto {
    const streamingSource = source.streamingSource;
    const response: StreamingInfoResponseDto = {
      type: 'direct',
      sourceName: streamingSource.displayName || streamingSource.name,
      translation: source.translation,
      availableQualities: source.availableQualities,
    };

    switch (streamingSource.type) {
      case StreamingSourceType.MINIO:
        response.type = 'minio';
        response.url = source.minioPath
          ? `${config.minio.publicUrl}/${config.minio.bucket}/${source.minioPath}`
          : null;
        break;

      case StreamingSourceType.DIRECT_URL:
        response.type = 'direct';
        response.url = source.directUrl;
        break;

      case StreamingSourceType.EXTERNAL_API:
        if (streamingSource.requiresProxy) {
          response.type = 'proxy';
          response.proxyEndpoint = `/streaming/proxy/${source.id}`;
        } else if (source.embedUrl) {
          response.type = 'embed';
          response.embedUrl = source.embedUrl;
        } else {
          response.type = 'direct';
          response.url = source.directUrl;
        }
        break;
    }

    return response;
  }

  // ==================== Video Streaming ====================

  /**
   * Stream video from MinIO
   */
  async streamFromMinio(
    objectPath: string,
    response: Response,
    range?: string,
  ): Promise<void> {
    try {
      const stat = await this.minioClient.statObject(config.minio.bucket, objectPath);
      const fileSize = stat.size;

      if (range) {
        // Handle range request for seeking
        const parts = range.replace(/bytes=/, '').split('-');
        const start = parseInt(parts[0], 10);
        const end = parts[1] ? parseInt(parts[1], 10) : Math.min(start + config.streaming.chunkSize, fileSize - 1);
        const chunkSize = end - start + 1;

        response.status(206);
        response.setHeader('Content-Range', `bytes ${start}-${end}/${fileSize}`);
        response.setHeader('Accept-Ranges', 'bytes');
        response.setHeader('Content-Length', chunkSize);
        response.setHeader('Content-Type', 'video/mp4');

        const stream = await this.minioClient.getPartialObject(
          config.minio.bucket,
          objectPath,
          start,
          chunkSize,
        );
        stream.pipe(response);
      } else {
        // Full file stream
        response.setHeader('Content-Length', fileSize);
        response.setHeader('Content-Type', 'video/mp4');
        response.setHeader('Accept-Ranges', 'bytes');

        const stream = await this.minioClient.getObject(config.minio.bucket, objectPath);
        stream.pipe(response);
      }
    } catch (error) {
      this.logger.error(`MinIO stream error: ${error.message}`);
      throw new HttpException('Video not found', HttpStatus.NOT_FOUND);
    }
  }

  /**
   * Proxy stream from external API
   */
  async proxyStream(
    sourceId: number,
    response: Response,
    range?: string,
  ): Promise<void> {
    const source = await this.animeSourceRepo.findOne({
      where: { id: sourceId },
      relations: ['streamingSource'],
    });

    if (!source || !source.directUrl) {
      throw new HttpException('Stream source not found', HttpStatus.NOT_FOUND);
    }

    try {
      const headers: Record<string, string> = {};
      if (range) {
        headers['Range'] = range;
      }

      const proxyResponse = await axios({
        method: 'GET',
        url: source.directUrl,
        responseType: 'stream',
        headers,
        timeout: config.streaming.externalTimeout,
      });

      // Forward headers
      const contentType = proxyResponse.headers['content-type'];
      const contentLength = proxyResponse.headers['content-length'];
      const contentRange = proxyResponse.headers['content-range'];
      const acceptRanges = proxyResponse.headers['accept-ranges'];

      if (contentType) response.setHeader('Content-Type', contentType);
      if (contentLength) response.setHeader('Content-Length', contentLength);
      if (contentRange) response.setHeader('Content-Range', contentRange);
      if (acceptRanges) response.setHeader('Accept-Ranges', acceptRanges);

      response.status(proxyResponse.status);
      proxyResponse.data.pipe(response);
    } catch (error) {
      this.logger.error(`Proxy stream error: ${error.message}`);
      throw new HttpException('Failed to proxy stream', HttpStatus.BAD_GATEWAY);
    }
  }

  /**
   * Get stream by source ID
   */
  async getStreamById(
    sourceId: number,
    response: Response,
    range?: string,
  ): Promise<void> {
    const source = await this.animeSourceRepo.findOne({
      where: { id: sourceId },
      relations: ['streamingSource'],
    });

    if (!source) {
      throw new HttpException('Stream source not found', HttpStatus.NOT_FOUND);
    }

    switch (source.streamingSource.type) {
      case StreamingSourceType.MINIO:
        if (!source.minioPath) {
          throw new HttpException('MinIO path not configured', HttpStatus.NOT_FOUND);
        }
        return this.streamFromMinio(source.minioPath, response, range);

      case StreamingSourceType.EXTERNAL_API:
      case StreamingSourceType.DIRECT_URL:
        if (source.streamingSource.requiresProxy || !source.directUrl) {
          return this.proxyStream(sourceId, response, range);
        }
        // Redirect to direct URL
        response.redirect(source.directUrl);
        return;

      default:
        throw new HttpException('Unknown streaming source type', HttpStatus.BAD_REQUEST);
    }
  }

  // ==================== MinIO Upload ====================

  /**
   * Upload video to MinIO
   */
  async uploadToMinio(
    animeId: number,
    file: Express.Multer.File,
    episode?: number,
    season?: number,
    translation?: string,
    quality?: VideoQuality,
  ): Promise<AnimeSource> {
    const anime = await this.animeRepo.findOne({ where: { id: animeId } });
    if (!anime) {
      throw new HttpException('Anime not found', HttpStatus.NOT_FOUND);
    }

    // Get or create MinIO source
    let minioSource = await this.streamingSourceRepo.findOne({
      where: { name: 'minio' },
    });

    if (!minioSource) {
      await this.initializeDefaultSources();
      minioSource = await this.streamingSourceRepo.findOne({
        where: { name: 'minio' },
      });
    }

    // Create bucket if not exists
    const bucketExists = await this.minioClient.bucketExists(config.minio.bucket);
    if (!bucketExists) {
      await this.minioClient.makeBucket(config.minio.bucket);
    }

    // Generate unique path
    const timestamp = Date.now();
    const sanitizedName = anime.name.replace(/[^a-zA-Z0-9]/g, '_');
    const episodePart = episode ? `_ep${episode}` : '';
    const seasonPart = season ? `_s${season}` : '';
    const objectPath = `${sanitizedName}${seasonPart}${episodePart}_${timestamp}.mp4`;

    // Upload to MinIO
    await this.minioClient.putObject(
      config.minio.bucket,
      objectPath,
      file.buffer,
      file.size,
      { 'Content-Type': 'video/mp4' },
    );

    // Create anime source entry
    const animeSource = this.animeSourceRepo.create({
      animeId,
      streamingSourceId: minioSource.id,
      episode,
      season,
      translation,
      quality: quality || VideoQuality.AUTO,
      minioPath: objectPath,
      active: true,
    });

    return this.animeSourceRepo.save(animeSource);
  }

  // ==================== External API Integration ====================

  /**
   * Search anime on external APIs
   * Returns streaming info that frontend can use directly or backend can proxy
   */
  async searchExternalSources(dto: ExternalApiSearchDto): Promise<{
    source: string;
    results: Array<{
      externalId: string;
      title: string;
      episodes?: number;
      translations: Array<{
        name: string;
        type: string;
        embedUrl?: string;
        directUrl?: string;
      }>;
    }>;
  }[]> {
    const results: Array<{
      source: string;
      results: Array<{
        externalId: string;
        title: string;
        episodes?: number;
        translations: Array<{
          name: string;
          type: string;
          embedUrl?: string;
          directUrl?: string;
        }>;
      }>;
    }> = [];

    // Search Kodik if enabled
    if (config.externalApis.kodik?.enabled && this.externalApis.has('kodik')) {
      try {
        const kodikResults = await this.searchKodik(dto);
        results.push({ source: 'kodik', results: kodikResults });
      } catch (error) {
        this.logger.warn(`Kodik search failed: ${error.message}`);
      }
    }

    // Search Anilibria if enabled
    if (config.externalApis.anilibria?.enabled && this.externalApis.has('anilibria')) {
      try {
        const anilibriaResults = await this.searchAnilibria(dto);
        results.push({ source: 'anilibria', results: anilibriaResults });
      } catch (error) {
        this.logger.warn(`Anilibria search failed: ${error.message}`);
      }
    }

    return results;
  }

  /**
   * Search Kodik API
   */
  private async searchKodik(dto: ExternalApiSearchDto): Promise<Array<{
    externalId: string;
    title: string;
    episodes?: number;
    translations: Array<{
      name: string;
      type: string;
      embedUrl?: string;
      directUrl?: string;
    }>;
  }>> {
    const api = this.externalApis.get('kodik');
    const params: Record<string, any> = {
      token: config.externalApis.kodik.apiKey,
      title: dto.query,
      with_episodes: true,
    };

    if (dto.shikimoriId) {
      params.shikimori_id = dto.shikimoriId;
    }

    const response = await api.get('/search', { params });
    const data = response.data;

    if (!data.results || !Array.isArray(data.results)) {
      return [];
    }

    return data.results.map((item: any) => ({
      externalId: item.id,
      title: item.title || item.title_orig,
      episodes: item.last_episode,
      translations: [{
        name: item.translation?.title || 'Unknown',
        type: item.translation?.type || 'voice',
        embedUrl: item.link,
      }],
    }));
  }

  /**
   * Search Anilibria API
   */
  private async searchAnilibria(dto: ExternalApiSearchDto): Promise<Array<{
    externalId: string;
    title: string;
    episodes?: number;
    translations: Array<{
      name: string;
      type: string;
      embedUrl?: string;
      directUrl?: string;
    }>;
  }>> {
    const api = this.externalApis.get('anilibria');
    const response = await api.get('/title/search', {
      params: { search: dto.query },
    });

    const data = response.data;
    if (!data.list || !Array.isArray(data.list)) {
      return [];
    }

    return data.list.map((item: any) => {
      const hls = item.player?.host && item.player?.list
        ? Object.values(item.player.list).map((ep: any) => ({
            episode: ep.episode,
            hls: ep.hls ? {
              sd: `https://${item.player.host}${ep.hls.sd}`,
              hd: `https://${item.player.host}${ep.hls.hd}`,
              fhd: ep.hls.fhd ? `https://${item.player.host}${ep.hls.fhd}` : null,
            } : null,
          }))
        : [];

      return {
        externalId: item.id?.toString(),
        title: item.names?.en || item.names?.ru,
        episodes: item.player?.episodes?.last,
        translations: [{
          name: 'AniLibria',
          type: 'voice',
          directUrl: hls.length > 0 ? hls[0]?.hls?.hd : null,
        }],
        episodesList: hls,
      };
    });
  }

  /**
   * Import external source to local DB
   */
  async importExternalSource(
    animeId: number,
    sourceType: string,
    externalId: string,
    translation: string,
    embedUrl?: string,
    directUrl?: string,
  ): Promise<AnimeSource> {
    // Find or create streaming source for external API
    let streamingSource = await this.streamingSourceRepo.findOne({
      where: { name: sourceType },
    });

    if (!streamingSource) {
      streamingSource = await this.createStreamingSource({
        name: sourceType,
        displayName: sourceType.charAt(0).toUpperCase() + sourceType.slice(1),
        type: StreamingSourceType.EXTERNAL_API,
        requiresProxy: !!directUrl, // Proxy if we have direct URL, otherwise embed
        priority: 50,
      });
    }

    const animeSource = this.animeSourceRepo.create({
      animeId,
      streamingSourceId: streamingSource.id,
      externalId,
      translation,
      embedUrl,
      directUrl,
      active: true,
    });

    return this.animeSourceRepo.save(animeSource);
  }
}
