import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, ManyToMany, DeleteDateColumn } from 'typeorm';
import { GenresAnimeEntity } from '../../genresAnime/entity/genresAnime.entity'

@Entity({
    name: "genres"
})
export class GenresEntity {
    @PrimaryGeneratedColumn()
    id: number

    @Column()
    name: string

    @Column()
    nameRu: string

    @Column()
    active: boolean

    @CreateDateColumn()
    createdAt: Date

    @UpdateDateColumn()
    updatedAt: Date

    @DeleteDateColumn()
    deleteAt: Date
}