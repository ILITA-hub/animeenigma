import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, OneToMany } from 'typeorm';
import { AnimeCollectionOpenings } from "./animeCollectionsOpenings.entity"

@Entity({
  name: "animeCollections"
})
export class AnimeCollections {
  @PrimaryGeneratedColumn()
  id: number

  @Column()
  name: String

  @Column()
  description: String

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

  @OneToMany(type => AnimeCollectionOpenings, openings => openings.animeCollection)
  openings: AnimeCollectionOpenings
}
