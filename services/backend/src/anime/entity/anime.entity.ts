import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, ManyToMany } from 'typeorm';
import { ObjectType, Field, Int } from '@nestjs/graphql'

@Entity({
  name: "anime"
})
@ObjectType()
export class AnimeEntity {
  // @Field({ nullable: true})
  @PrimaryGeneratedColumn()
  id: number

  @Column({ nullable: false})
  name: string

  @Column({ nullable: true })
  nameRU: string

  @Column({ nullable: true})
  nameJP: string

  // @Field({ nullable: false})
  // @Column()
  // description: string

  // @Field({ nullable: false})
  // @Column()
  // imgPath: string

  @Column()
  active: boolean

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date
}
