import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, DeleteDateColumn, Unique } from 'typeorm';

@Entity({
  name: "room"
})
export class RoomEntity {
  @PrimaryGeneratedColumn()
  id: number

  @Column()
  name: String

  @Column()
  maxPlayer: Number

  @Column()
  port: Number

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

  @DeleteDateColumn()
  deleteAt: Date
}

@Entity({
  name: "roomOpenings"
})
export class RoomOpeningsEntity {
  @PrimaryGeneratedColumn()
  id: Number

  @Column()
  idRoom: Number

  @Column()
  type: String

  @Column()
  idEntity: Number

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

  @DeleteDateColumn()
  deleteAt: Date
}