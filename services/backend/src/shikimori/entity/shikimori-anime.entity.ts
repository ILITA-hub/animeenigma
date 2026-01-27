import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, OneToOne, JoinColumn, Index } from 'typeorm';
import { AnimeEntity } from '../../anime/entity/anime.entity';

export enum ShikimoriAnimeKind {
  TV = 'tv',
  MOVIE = 'movie',
  OVA = 'ova',
  ONA = 'ona',
  SPECIAL = 'special',
  TV_SPECIAL = 'tv_special',
  MUSIC = 'music',
  PV = 'pv',
  CM = 'cm',
}

export enum ShikimoriAnimeStatus {
  ANONS = 'anons',
  ONGOING = 'ongoing',
  RELEASED = 'released',
}

@Entity({ name: 'shikimori_anime' })
export class ShikimoriAnime {
  @PrimaryGeneratedColumn()
  id: number;

  @Column({ unique: true })
  @Index()
  shikimoriId: number; // ID from Shikimori API

  @Column({ nullable: true })
  animeId: number; // Local anime ID (mapped)

  @OneToOne(() => AnimeEntity, { nullable: true, onDelete: 'SET NULL' })
  @JoinColumn({ name: 'animeId' })
  anime: AnimeEntity;

  @Column()
  @Index()
  name: string; // Original (romaji) name - used for keying

  @Column({ nullable: true })
  nameRussian: string;

  @Column({ nullable: true })
  nameEnglish: string;

  @Column({ nullable: true })
  nameJapanese: string;

  @Column({ type: 'text', array: true, nullable: true })
  synonyms: string[]; // Alternative names for search

  @Column({
    type: 'enum',
    enum: ShikimoriAnimeKind,
    nullable: true,
  })
  kind: ShikimoriAnimeKind;

  @Column({
    type: 'enum',
    enum: ShikimoriAnimeStatus,
    nullable: true,
  })
  status: ShikimoriAnimeStatus;

  @Column({ nullable: true })
  episodes: number; // Total episodes

  @Column({ nullable: true })
  episodesAired: number; // Episodes aired so far

  @Column({ type: 'date', nullable: true })
  airedOn: Date;

  @Column({ type: 'date', nullable: true })
  releasedOn: Date;

  @Column({ type: 'float', nullable: true })
  score: number;

  @Column({ nullable: true })
  rating: string; // e.g., 'pg_13', 'r'

  @Column({ nullable: true })
  duration: number; // Episode duration in minutes

  @Column({ type: 'text', nullable: true })
  description: string;

  @Column({ nullable: true })
  posterUrl: string; // Poster image URL from Shikimori

  @Column({ nullable: true })
  localPosterPath: string; // Cached poster path (MinIO or local)

  @Column({ type: 'jsonb', nullable: true })
  genres: Array<{ id: number; name: string; russian: string }>;

  @Column({ type: 'jsonb', nullable: true })
  studios: Array<{ id: number; name: string }>;

  @Column({ type: 'jsonb', nullable: true })
  rawData: Record<string, any>; // Full raw response from Shikimori

  @Column({ default: false })
  isMapped: boolean; // Whether this has been mapped to local anime

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;

  @Column({ nullable: true })
  lastSyncedAt: Date; // Last time data was synced from Shikimori
}
