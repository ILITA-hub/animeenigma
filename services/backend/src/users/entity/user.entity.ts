
import { Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn } from 'typeorm';

@Entity({
  name: "users"
})
export class UserEntity {
  @PrimaryGeneratedColumn()
  id: Number
  
  @Column()
  username: String

  @Column({ unique: true })
  login : String

  @Column()
  password : String

  @CreateDateColumn()
  createdAt: Date

  @UpdateDateColumn()
  updatedAt: Date

}
