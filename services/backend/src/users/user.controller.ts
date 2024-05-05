import { Controller, Get, Post, Param, Body, Delete, Res, HttpException, HttpCode } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { UserService } from './user.service'
import { UserDto, UserDtoReg, UserDTOLogout } from './dto/user.dto'
import { UserLoginDto } from './dto/userLogin.dto'
import * as bcrypt from 'bcrypt'
import { BadRequestSchema400, SucsessfulRequest200 } from './schema/request.schema'

@ApiTags("user")
@Controller("users")
export class UserController {
  constructor(private readonly userService: UserService) {}

  // @Post("/login") // TODO PEREDELAT' КОГДА СДЕЛАЕМ АУТИСТИФИКАЦИЮ
  // async getUserSessionByName(@Body() userLoginDto: UserLoginDto) {
  //   console.log(userLoginDto.name)
  //   let user = await this.userService.getUserByName(userLoginDto.name);

  //   if (!user) {
  //     user = await this.userService.createUser(userLoginDto);
  //   }

  //   const sessionId = await this.userService.createUserSession(user);
  //   return { sessionId, userData: user };
  // }

  @Post("/login")
  @ApiResponse({status : 400, description: "Ошибка в параметрах", type: BadRequestSchema400})
  @ApiResponse({status : 200, description: "Авторизация успешна пройдена", type: SucsessfulRequest200})
  @HttpCode(200)
  async login(@Body() userDto: UserDto) {
    const result = await this.userService.loginUser(userDto)

    return {token : result}
  }

  @Post("/reg")
  @ApiResponse({status : 400, description: "Ошибка в параметрах", type: BadRequestSchema400})
  @ApiResponse({status : 200, description: "Регистрация прошла успешно", type: SucsessfulRequest200})
  @HttpCode(200)
  async reg(@Body() userDTO: UserDtoReg) {
    const result = await this.userService.createUser(userDTO)
    
    return {token : result}
  }

  @Post("/logout")
  @HttpCode(200)
  async logout(@Body() sessionId : UserDTOLogout) {
    await this.userService.logoutUser(sessionId.sessionId)
  }
}
