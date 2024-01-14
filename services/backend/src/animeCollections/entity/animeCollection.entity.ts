import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, OneToMany } from 'typeorm';

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
}
