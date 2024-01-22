import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, ManyToMany } from 'typeorm';
import { AnimeEntity } from '../../anime/entity/anime.entity'
import { GenresEntity } from '../../genres/entity/genres.entity'

@Entity({
    name: "genresAnime"
})
export class GenresAnimeEntity {
    @PrimaryGeneratedColumn()
    id: number

    @ManyToOne(() => AnimeEntity, anime => anime.id)
    anime: number

    @Column()
    active: boolean

    @ManyToOne(() => GenresEntity, genres => genres.id)
    genre: number

    @CreateDateColumn()
    createdAt: Date

    @UpdateDateColumn()
    updatedAt: Date
}