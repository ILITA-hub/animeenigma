import { Injectable } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';
import { Strategy } from 'passport-jwt';
import { config } from '../../config/index';
import { UserService } from '../../users/user.service';

@Injectable()
export class SocketStrategy extends PassportStrategy(Strategy, 'socket') {
  constructor(private readonly userService: UserService) {
    super({
      jwtFromRequest: (request) => {
        return request?.handshake?.headers?.authorization
      },
      ignoreExpiration: false,
      secretOrKey: config.jwtSecret,
    });
  }

  async validate(payload: any) {
    // console.log(payload)
    // const user = await this.userService.findOne(payload.username);
    // return user; //{ username: payload.username };
  }
}