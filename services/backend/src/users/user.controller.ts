import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common'
import { ApiBody, ApiResponse } from '@nestjs/swagger'
import { UserService } from './user.service'
import { UserDto } from './dto/user.dto'
import { UserLoginDto } from './dto/userLogin.dto'

@Controller("users")
export class UserController {
  constructor(private readonly userService: UserService) {}

  @Post("/login") // TODO PEREDELAT' КОГДА СДЕЛАЕМ АУТИСТИФИКАЦИЮ
  async getUserSessionByName(@Body() userLoginDto: UserLoginDto) {
    console.log(userLoginDto.name)
    let user = await this.userService.getUserByName(userLoginDto.name);

    if (!user) {
      user = await this.userService.createUser(userLoginDto);
    }

    const sessionId = await this.userService.createUserSession(user);
    return { sessionId, userData: user };
  }

  // @Post()
  // async create(@Body() userDto: UserDto) {
  //   return await this.userService.createUser(userDto);
  // }
}
