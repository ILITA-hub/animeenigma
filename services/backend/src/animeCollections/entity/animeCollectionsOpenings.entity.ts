import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne } from 'typeorm';
import { AnimeCollections } from './animeCollection.entity'
import { VideosEntity } from '../../videos/entity/videos.entity'

@Entity({
  name: "animeCollectionOpenings"
})
export class AnimeCollectionOpenings {
  @PrimaryGeneratedColumn()
  id: number

  @ManyToOne(() => AnimeCollections, animeCollection => animeCollection.id)
  animeCollection: number

  @ManyToOne(() => VideosEntity, animeOpening => animeOpening.id)
  animeOpening: number

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date
}
