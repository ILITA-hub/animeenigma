import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, OneToMany, ManyToOne } from 'typeorm';
import { AnimeCollectionOpenings } from "./animeCollectionsOpenings.entity"
import { UserEntity } from "../../users/entity/user.entity"

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

  @OneToMany(() => AnimeCollectionOpenings, openings => openings.animeCollection)
  openings: AnimeCollectionOpenings

  @ManyToOne(() => UserEntity, owner => owner.collections)
  owner: UserEntity
}
