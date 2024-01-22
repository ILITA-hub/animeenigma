import { Injectable } from '@nestjs/common';
import { generateRoomId, uuidv4 } from '../utils/miscellaneous'
import { CachesService } from '../caches/caches.service'
import { UserDto } from './dto/user.dto'
import { Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { UserEntity } from './entity/user.entity';
import { UserLoginDto } from './dto/userLogin.dto';

@Injectable()
export class UserService {
  constructor(
    @InjectRepository(UserEntity) private readonly UserRepository: Repository<UserEntity>,
    private readonly cachesService: CachesService,
  ) {}

  async getUserById(userId: number) {
    
    const user = await this.UserRepository.findOne({
      where: {
        id: userId
      }
    })

    return user;

  }

  async getUserByName(userName: string) {

    const user = await this.UserRepository.findOne({
      where: {
        name: userName
      }
    })

    return user;

  }

  async createUser(userDto: UserLoginDto) {

    const user = await this.UserRepository.save({
      name: userDto.name,
    })

    return user;
  }

  async getUserSession(sessionId: string) {
    const session = await this.cachesService.getCache('userSession' + sessionId);
    return session;
  }

  async createUserSession(userEntity: UserEntity) {
    const sessionId = uuidv4();
    await this.cachesService.setCache('userSession' + sessionId, {
      userId: userEntity.id,
    });
    return sessionId;
  }

}
