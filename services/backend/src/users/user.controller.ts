import { Controller, Get, Post, Param, Body, Delete, Res, HttpException, HttpCode } from '@nestjs/common'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'
import { UserService } from './user.service'
import { UserDto, UserDtoReg } from './dto/user.dto'
import { UserLoginDto } from './dto/userLogin.dto'
import { BadRequestSchema401, SucsessfulRequest200 } from './schema/request.schema'

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
  @ApiResponse({status : 400, description: "Ошибка в параметрах", type: BadRequestSchema401})
  @ApiResponse({status : 200, description: "Ошибка в параметрах", type: SucsessfulRequest200})
  @HttpCode(200)
  async login(@Body() userDto: UserDto) {
    const result = await this.userService.loginUser(userDto)
    if (result == null) {
      throw new HttpException("user or password incorrect", 401)
    }
    return {token : result}
  }

  @Post("/reg")
  async reg(@Body() userDTO: UserDtoReg) {
    const result = await this.userService.createUser(userDTO)
  }
}
