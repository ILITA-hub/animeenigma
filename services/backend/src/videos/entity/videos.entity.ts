import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany } from 'typeorm';
import { AnimeEntity } from '../../anime/entity/anime.entity'
import { AnimeCollectionOpenings } from '../../animeCollections/entity/animeCollectionsOpenings.entity'

@Entity({
  name: "videos"
})
export class VideosEntity {
  @PrimaryGeneratedColumn()
  id: number

  @Column()
  active: boolean

  @ManyToOne(() => AnimeEntity, anime => anime.id)
  anime: number

  @Column({nullable: true})
  mp4Path: string

  @Column()
  name: string

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

  @Column()
  kind: string
}
