import { Injectable, Logger, HttpException, HttpStatus } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, Like, ILike } from 'typeorm';
import axios, { AxiosInstance } from 'axios';
import { config } from '../config';
import { ShikimoriAnime, ShikimoriAnimeKind, ShikimoriAnimeStatus } from './entity/shikimori-anime.entity';
import { AnimeEntity } from '../anime/entity/anime.entity';
import { CachesService } from '../caches/caches.service';
import { SearchAnimeDto, ShikimoriAnimeResponseDto } from './dto/shikimori.dto';

interface ShikimoriApiAnime {
  id: number;
  name: string;
  russian: string;
  english: string[];
  japanese: string[];
  synonyms: string[];
  kind: string;
  status: string;
  episodes: number;
  episodes_aired: number;
  aired_on: string;
  released_on: string;
  score: string;
  rating: string;
  duration: number;
  description: string;
  genres: Array<{ id: number; name: string; russian: string }>;
  studios: Array<{ id: number; name: string }>;
  image: {
    original: string;
    preview: string;
    x96: string;
    x48: string;
  };
}

@Injectable()
export class ShikimoriService {
  private readonly logger = new Logger(ShikimoriService.name);
  private readonly api: AxiosInstance;
  private lastRequestTime = 0;

  constructor(
    @InjectRepository(ShikimoriAnime)
    private readonly shikimoriAnimeRepo: Repository<ShikimoriAnime>,
    @InjectRepository(AnimeEntity)
    private readonly animeRepo: Repository<AnimeEntity>,
    private readonly cacheService: CachesService,
  ) {
    this.api = axios.create({
      baseURL: config.shikimori.baseUrl,
      headers: {
        'User-Agent': config.shikimori.userAgent,
        'Accept': 'application/json',
      },
      timeout: 10000,
    });
  }

  /**
   * Rate limiter for Shikimori API
   */
  private async rateLimit(): Promise<void> {
    const now = Date.now();
    const minInterval = 1000 / config.shikimori.rateLimit;
    const elapsed = now - this.lastRequestTime;

    if (elapsed < minInterval) {
      await new Promise(resolve => setTimeout(resolve, minInterval - elapsed));
    }
    this.lastRequestTime = Date.now();
  }

  /**
   * Search anime on Shikimori API and cache results locally
   */
  async searchAnime(dto: SearchAnimeDto): Promise<ShikimoriAnimeResponseDto[]> {
    const cacheKey = `shikimori:search:${JSON.stringify(dto)}`;

    // Check cache first
    const cached = await this.cacheService.getCache(cacheKey);
    if (cached) {
      return JSON.parse(cached);
    }

    // Check local DB first for existing matches
    const localMatches = await this.shikimoriAnimeRepo.find({
      where: [
        { name: ILike(`%${dto.query}%`) },
        { nameRussian: ILike(`%${dto.query}%`) },
        { nameEnglish: ILike(`%${dto.query}%`) },
      ],
      take: dto.limit,
      skip: (dto.page - 1) * dto.limit,
      relations: ['anime'],
    });

    // If we have enough local results, return them
    if (localMatches.length >= dto.limit) {
      const results = this.formatResults(localMatches);
      await this.cacheService.setCache(cacheKey, JSON.stringify(results), config.shikimori.cacheTTL);
      return results;
    }

    // Fetch from Shikimori API
    try {
      await this.rateLimit();

      const params: Record<string, any> = {
        search: dto.query,
        page: dto.page,
        limit: dto.limit,
      };

      if (dto.kind) params.kind = dto.kind;
      if (dto.status) params.status = dto.status;
      if (dto.year) params.season = dto.year;

      const response = await this.api.get<ShikimoriApiAnime[]>('/animes', { params });

      // Save new anime to local DB
      const savedAnime = await Promise.all(
        response.data.map(async (anime) => {
          return this.saveOrUpdateShikimoriAnime(anime);
        })
      );

      const results = this.formatResults(savedAnime);
      await this.cacheService.setCache(cacheKey, JSON.stringify(results), config.shikimori.cacheTTL);

      return results;
    } catch (error) {
      this.logger.error(`Shikimori API error: ${error.message}`);

      // Return local results if API fails
      if (localMatches.length > 0) {
        return this.formatResults(localMatches);
      }

      throw new HttpException(
        'Failed to fetch anime from Shikimori',
        HttpStatus.SERVICE_UNAVAILABLE
      );
    }
  }

  /**
   * Get detailed anime info by Shikimori ID
   */
  async getAnimeById(shikimoriId: number): Promise<ShikimoriAnime> {
    // Check local DB first
    let anime = await this.shikimoriAnimeRepo.findOne({
      where: { shikimoriId },
      relations: ['anime'],
    });

    // Check if we need to refresh from API
    const shouldRefresh = !anime ||
      !anime.lastSyncedAt ||
      (Date.now() - anime.lastSyncedAt.getTime()) > config.shikimori.cacheTTL * 1000;

    if (shouldRefresh) {
      try {
        await this.rateLimit();
        const response = await this.api.get<ShikimoriApiAnime>(`/animes/${shikimoriId}`);
        anime = await this.saveOrUpdateShikimoriAnime(response.data, true);
      } catch (error) {
        this.logger.error(`Failed to fetch anime ${shikimoriId}: ${error.message}`);
        if (!anime) {
          throw new HttpException('Anime not found', HttpStatus.NOT_FOUND);
        }
      }
    }

    return anime;
  }

  /**
   * Map Shikimori anime to local anime entity
   * Creates a new local anime if localAnimeId is not provided
   */
  async mapToLocalAnime(shikimoriId: number, localAnimeId?: number): Promise<ShikimoriAnime> {
    const shikimoriAnime = await this.getAnimeById(shikimoriId);

    if (shikimoriAnime.isMapped && shikimoriAnime.animeId) {
      // Already mapped, return existing
      return shikimoriAnime;
    }

    let localAnime: AnimeEntity;

    if (localAnimeId) {
      // Map to existing local anime
      localAnime = await this.animeRepo.findOne({ where: { id: localAnimeId } });
      if (!localAnime) {
        throw new HttpException('Local anime not found', HttpStatus.NOT_FOUND);
      }
    } else {
      // Create new local anime from Shikimori data
      localAnime = this.animeRepo.create({
        name: shikimoriAnime.name,
        nameRU: shikimoriAnime.nameRussian,
        nameJP: shikimoriAnime.nameJapanese,
        year: shikimoriAnime.airedOn ? new Date(shikimoriAnime.airedOn).getFullYear() : null,
        imgPath: shikimoriAnime.posterUrl,
        active: true,
      });
      localAnime = await this.animeRepo.save(localAnime);
    }

    // Update Shikimori anime mapping
    shikimoriAnime.animeId = localAnime.id;
    shikimoriAnime.anime = localAnime;
    shikimoriAnime.isMapped = true;

    return this.shikimoriAnimeRepo.save(shikimoriAnime);
  }

  /**
   * Get all mapped anime (for streaming)
   */
  async getMappedAnime(page = 1, limit = 20): Promise<{ data: ShikimoriAnime[]; total: number }> {
    const [data, total] = await this.shikimoriAnimeRepo.findAndCount({
      where: { isMapped: true },
      relations: ['anime'],
      order: { updatedAt: 'DESC' },
      skip: (page - 1) * limit,
      take: limit,
    });

    return { data, total };
  }

  /**
   * Save or update Shikimori anime in local DB
   */
  private async saveOrUpdateShikimoriAnime(
    apiAnime: ShikimoriApiAnime,
    fullData = false
  ): Promise<ShikimoriAnime> {
    let anime = await this.shikimoriAnimeRepo.findOne({
      where: { shikimoriId: apiAnime.id },
      relations: ['anime'],
    });

    const posterUrl = apiAnime.image?.original
      ? `https://shikimori.one${apiAnime.image.original}`
      : null;

    const data: Partial<ShikimoriAnime> = {
      shikimoriId: apiAnime.id,
      name: apiAnime.name,
      nameRussian: apiAnime.russian,
      nameEnglish: apiAnime.english?.[0],
      nameJapanese: apiAnime.japanese?.[0],
      synonyms: apiAnime.synonyms,
      kind: apiAnime.kind as ShikimoriAnimeKind,
      status: apiAnime.status as ShikimoriAnimeStatus,
      episodes: apiAnime.episodes,
      episodesAired: apiAnime.episodes_aired,
      score: parseFloat(apiAnime.score) || null,
      posterUrl,
      genres: apiAnime.genres,
      studios: apiAnime.studios,
      lastSyncedAt: new Date(),
    };

    if (fullData) {
      data.airedOn = apiAnime.aired_on ? new Date(apiAnime.aired_on) : null;
      data.releasedOn = apiAnime.released_on ? new Date(apiAnime.released_on) : null;
      data.rating = apiAnime.rating;
      data.duration = apiAnime.duration;
      data.description = apiAnime.description;
      data.rawData = apiAnime;
    }

    if (anime) {
      Object.assign(anime, data);
    } else {
      anime = this.shikimoriAnimeRepo.create(data);
    }

    return this.shikimoriAnimeRepo.save(anime);
  }

  /**
   * Format results for API response
   */
  private formatResults(anime: ShikimoriAnime[]): ShikimoriAnimeResponseDto[] {
    return anime.map(a => ({
      id: a.id,
      shikimoriId: a.shikimoriId,
      name: a.name,
      nameRussian: a.nameRussian,
      nameEnglish: a.nameEnglish,
      nameJapanese: a.nameJapanese,
      kind: a.kind,
      status: a.status,
      episodes: a.episodes,
      episodesAired: a.episodesAired,
      score: a.score,
      posterUrl: a.posterUrl,
      description: a.description,
      genres: a.genres,
      isMapped: a.isMapped,
      localAnimeId: a.animeId,
    }));
  }
}
