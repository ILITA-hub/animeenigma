import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common'
import { ApiBody, ApiResponse } from '@nestjs/swagger'
import { UserService } from './user.service'
import { UserDto } from './dto/user.dto'

@Controller("users")
export class UserController {
  constructor(private readonly userService: UserService) {}

  @Get(":name") // TODO УБРАТЬ КОГДА СДЕЛАЕМ АУТИСТИФИКАЦИЮ
  async getUserSessionByName(@Param() params) {
    const user = await this.userService.getUserByName(params.name);
    const sessionId = await this.userService.createUserSession(user);
    return sessionId;
  }

  @Post()
  async create(@Body() userDto: UserDto) {
    return await this.userService.createUser(userDto);
  }
}
