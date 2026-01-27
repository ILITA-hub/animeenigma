import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, JoinColumn, Index } from 'typeorm';
import { AnimeEntity } from '../../anime/entity/anime.entity';
import { StreamingSource } from './streaming-source.entity';

export enum VideoQuality {
  SD_360 = '360p',
  SD_480 = '480p',
  HD_720 = '720p',
  HD_1080 = '1080p',
  UHD_4K = '4k',
  AUTO = 'auto',
}

@Entity({ name: 'anime_sources' })
@Index(['animeId', 'streamingSourceId', 'episode'], { unique: true })
export class AnimeSource {
  @PrimaryGeneratedColumn()
  id: number;

  @Column()
  animeId: number;

  @ManyToOne(() => AnimeEntity, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'animeId' })
  anime: AnimeEntity;

  @Column()
  streamingSourceId: number;

  @ManyToOne(() => StreamingSource, (source) => source.animeSources, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'streamingSourceId' })
  streamingSource: StreamingSource;

  @Column({ nullable: true })
  externalId: string; // ID in the external system (e.g., Kodik ID, Shikimori ID)

  @Column({ nullable: true })
  episode: number; // Episode number, null for full series or movies

  @Column({ nullable: true })
  season: number; // Season number

  @Column({ nullable: true })
  translation: string; // Translation/dub name (e.g., 'AniLibria', 'JAM')

  @Column({ nullable: true })
  translationType: string; // 'voice', 'subtitles'

  @Column({ nullable: true })
  directUrl: string; // Direct stream URL (for DIRECT_URL type or cached)

  @Column({ nullable: true })
  embedUrl: string; // Embed player URL (for iframe embedding)

  @Column({ nullable: true })
  minioPath: string; // Path in MinIO bucket (for MINIO type)

  @Column({
    type: 'enum',
    enum: VideoQuality,
    default: VideoQuality.AUTO,
  })
  quality: VideoQuality;

  @Column({ type: 'jsonb', nullable: true })
  availableQualities: string[]; // List of available quality options

  @Column({ type: 'jsonb', nullable: true })
  metadata: Record<string, any>; // Additional metadata from external API

  @Column({ default: true })
  active: boolean;

  @Column({ nullable: true })
  lastChecked: Date; // When the source was last verified

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
