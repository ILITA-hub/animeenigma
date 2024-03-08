import { Injectable } from '@nestjs/common';
import { generateRoomId, uuidv4 } from '../utils/miscellaneous'
import { CachesService } from '../caches/caches.service'
import { UserDto, UserDtoReg } from './dto/user.dto'
import { Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { UserEntity } from './entity/user.entity';
import { UserLoginDto } from './dto/userLogin.dto';

@Injectable()
export class UserService {
  constructor(
    @InjectRepository(UserEntity) private readonly UserRepository: Repository<UserEntity>,
    private readonly cachesService: CachesService,
  ) { }

  async getUserById(userId: number) {

    const user = await this.UserRepository.findOne({
      where: {
        id: userId
      }
    })

    return user;

  }

  async loginUser(user: UserDto) {
    let userClass = await this.UserRepository.findOne({
      where : {
        login : user.login
      }
    })
    if (userClass == null) {
      return null
    }
    if (userClass.password != user.password) {
      return null
    }
    return "qwe"
  }

  async createUser(user: UserDtoReg) {

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
