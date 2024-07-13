import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, ManyToMany } from 'typeorm';
import { VideosEntity } from '../../videos/entity/videos.entity'
import { GenresAnimeEntity } from '../../genresAnime/entity/genresAnime.entity'

@Entity({
  name: "anime"
})
export class AnimeEntity {

  @PrimaryGeneratedColumn()
  id: number

  @Column({ nullable: false})
  name: string

  @Column({ nullable: true })
  nameRU: string

  @Column({ nullable: true})
  nameJP: string

  @Column()
  year: number

  // @Column({ nullable: false})
  // description: string

  @Column({ nullable: true })
  imgPath: string

  @Column()
  active: boolean

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

  @OneToMany(() => VideosEntity, videos => videos.anime)
  videos: VideosEntity

  @OneToMany(() => GenresAnimeEntity, genres => genres.anime)
  genres: GenresAnimeEntity
}
