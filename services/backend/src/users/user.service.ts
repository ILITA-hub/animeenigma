import { HttpException, Injectable } from '@nestjs/common';
import { generateRoomId, uuidv4 } from '../utils/miscellaneous'
import { CachesService } from '../caches/caches.service'
import { UserDto, UserDtoReg } from './dto/user.dto'
import { QueryFailedError, Repository } from 'typeorm';
import { InjectRepository } from '@nestjs/typeorm'
import { UserEntity } from './entity/user.entity';
import { UserLoginDto } from './dto/userLogin.dto';
import * as bcrypt from 'bcrypt'

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
    const userClass = await this.UserRepository.findOne({
      where: {
        login: user.login
      }
    })
    if (userClass == null) {
      throw new HttpException("user or password incorrect", 400) 
    }
    if (userClass.password != user.password) {
      throw new HttpException("user or password incorrect", 400)
    }

    return this.createUserSession(userClass)
  }

  async createUser(user: UserDtoReg) {
    if (user.password != user.confirmPassword) { 
      throw new HttpException("Пароли не совпадают", 400)
    }
    try {
      const userClass = await this.UserRepository.save({
        login: user.login,
        password: await bcrypt.hash(user.password, 10),
        username: user.username
      })
      return this.createUserSession(userClass)
    } catch (e) {
      if (e instanceof QueryFailedError) {
        throw new HttpException("Пользователь с таким логином уже существует", 400)
      }
    }
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

  getRandom(max: number) {
    return Math.floor(Math.random() * max)
  }

  createToken() {
    const sim = "qwertyuiopasdfghjkl[];{}|zxcvbnm,.<>/?1234567890-+=_!@#$%^&*()"
    let token = ""
    for (let i = 0; i < 50; i++) {
      token += sim[this.getRandom(sim.length)]
    }
    return token
  }

}
