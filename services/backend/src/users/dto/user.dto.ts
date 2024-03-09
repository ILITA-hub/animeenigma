import { IsString, IsInt, IsNotEmpty, IsArray } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';

class UserDto {
  @IsNotEmpty()
  @ApiProperty()
  login : String

  @IsNotEmpty()
  @ApiProperty()
  password : String
}

class UserDtoReg {
  @IsNotEmpty()
  @ApiProperty()
  username : String

  @IsNotEmpty()
  @ApiProperty()
  login : String

  @IsNotEmpty()
  @ApiProperty()
  password : String

  @IsNotEmpty()
  @ApiProperty()
  confirmPassword : String
}
export {UserDto, UserDtoReg}