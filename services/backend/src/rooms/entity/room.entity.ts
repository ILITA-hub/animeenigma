import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn, ManyToOne, OneToMany, DeleteDateColumn, Unique } from 'typeorm';

export enum RoomStatus {
  STARTING = "STARTING",
  PLAYING = "PLAYING",
  CLOUSING = "CLOUSING",
  OFFLINE = "OFFLINE"
}

@Entity({
  name: "room"
})
export class RoomEntity {
  @PrimaryGeneratedColumn()
  id: number

  @Column()
  name: string

  @Column()
  maxPlayer: number

  @Column()
  port: number
  
  @Column({
    type: "enum",
    enum: RoomStatus,
    default: RoomStatus.STARTING
  })
  status: RoomStatus

  @Column({ nullable: true })
  uniqueURL : string

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
  id: number

  @Column()
  idRoom: number

  @Column()
  type: string

  @Column()
  idEntity: number

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

  @DeleteDateColumn()
  deleteAt: Date
}