import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne } from 'typeorm';
import { AnimeCollections } from './animeCollection.entity'
import { VideosEntity } from '../../videos/entity/videos.entity'

@Entity({
  name: "animeCollectionOpenings"
})
export class AnimeCollectionOpenings {
  @PrimaryGeneratedColumn()
  id: number

  @ManyToOne(() => AnimeCollections, animeCollection => animeCollection.openings)
  animeCollection: AnimeCollections

  @ManyToOne(() => VideosEntity, animeOpening => animeOpening.animeCollections)
  animeOpening: VideosEntity

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date
}
