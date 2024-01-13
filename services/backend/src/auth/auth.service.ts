import { Injectable } from '@nestjs/common';
// import { UserService } from '../user/user.service';
import { JwtService } from '@nestjs/jwt';

@Injectable()
export class AuthService {
  constructor(
    private readonly userService: {}, // UserService
    private readonly jwtService: JwtService,
  ) {}

  private makeToken(): any{
    var text = "";
    var possible = "abcdefghijklmnopqrstuvwxyz";

    for( var i=0; i < 200; i++ )
        text += possible.charAt(Math.floor(Math.random() * possible.length));

    return text;
  }

  async validateUser(username: string, password: string): Promise<any> {
    // const user = await this.userService.findOneAuth(username);
    // if (user && user.password === password) {
    //   const { password, ...result } = user;
    //   return result;
    // }
    return null;
  }

  async login(username: string) {
    // const user = await this.userService.findOne(username);
    // const payload = { username: user.username };
    // return {
    //   access_token: this.jwtService.sign(payload),
    // };
  }

  async authorization (req: any) {
    const user_username = req.username
    const user_password = req.password

    const token = user_username + String(this.makeToken())

    return req;
  }
}