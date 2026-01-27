import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, OneToMany } from 'typeorm';
import { AnimeSource } from './anime-source.entity';

export enum StreamingSourceType {
  MINIO = 'minio',           // Self-hosted MinIO storage
  EXTERNAL_API = 'external', // External API (Kodik, Anilibria, etc.)
  DIRECT_URL = 'direct',     // Direct URL (for frontend direct access)
}

@Entity({ name: 'streaming_sources' })
export class StreamingSource {
  @PrimaryGeneratedColumn()
  id: number;

  @Column({ unique: true })
  name: string; // e.g., 'kodik', 'anilibria', 'minio'

  @Column({ nullable: true })
  displayName: string; // e.g., 'Kodik Player', 'Anilibria'

  @Column({
    type: 'enum',
    enum: StreamingSourceType,
    default: StreamingSourceType.EXTERNAL_API,
  })
  type: StreamingSourceType;

  @Column({ nullable: true })
  baseUrl: string; // Base URL for the API

  @Column({ nullable: true })
  apiKey: string; // API key if required

  @Column({ type: 'jsonb', nullable: true })
  config: Record<string, any>; // Additional configuration

  @Column({ default: true })
  active: boolean;

  @Column({ default: false })
  requiresProxy: boolean; // Whether backend needs to restream

  @Column({ default: 0 })
  priority: number; // Higher priority sources are tried first

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;

  @OneToMany(() => AnimeSource, (animeSource) => animeSource.streamingSource)
  animeSources: AnimeSource[];
}
