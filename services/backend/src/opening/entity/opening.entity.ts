import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany } from 'typeorm';
import { AnimeEntity } from '../../anime/entity/anime.entity'
import { AnimeCollectionOpenings } from '../../animeCollections/entity/animeCollectionsOpenings.entity'

@Entity({
  name: "openings"
})
export class OpeningsEntity {
  @PrimaryGeneratedColumn()
  id: number

  @Column()
  active: boolean

  @ManyToOne(() => AnimeEntity, animeId => animeId.id)
  anime: number

  @Column({
    nullable: true
  })
  mp3OpPath: string

  @Column()
  name: string

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date
}
