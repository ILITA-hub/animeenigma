import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, ManyToMany } from 'typeorm';
import { OpeningsEntity } from '../../opening/entity/opening.entity';
import { GenresAnimeEntity } from '../../genresAnime/entity/genresAnime.entity';
import { ObjectType, Field, Int } from '@nestjs/graphql'

@Entity({
  name: "anime"
})
@ObjectType()
export class AnimeEntity {
  @Field({ nullable: true})
  @PrimaryGeneratedColumn()
  id: number
  
  @Field({ nullable: true})
  @Column()
  name: string

  @Field({ nullable: true})
  @Column()
  nameRU: string

  @Field({ nullable: true})
  @Column()
  nameJP: string

  @Field({ nullable: true})
  @Column()
  description: string

  @Field({ nullable: true})
  @Column()
  imgPath: string

  @Field({ nullable: true})
  @Column()
  active: boolean

  @Field({ nullable: true})
  @CreateDateColumn()
  createdAt: Date

  @Field({ nullable: true})
  @UpdateDateColumn()
  updatedAt: Date
}
