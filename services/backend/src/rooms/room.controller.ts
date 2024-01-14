import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common';
import { RoomService } from './room.service';
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { BadRequestSchema } from './schema/badrequest.schema'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'

@ApiTags("Room")
@Controller("rooms")
export class AppController {
  constructor(private readonly appService: RoomService) {}

  @Get("getAll")
  async getAllRooms() {
    return await this.appService.getAllRooms();
  }

  @Get(":roomId")
  async getRoom(@Param("roomId") roomId : string) {
    const result = await this.appService.getRoom(roomId);
    if (result.status != 200) {
      throw new HttpException("", result.status);
    }
    return result.room
  }

  // @ApiBody({ type: SchemaRoom })
  @ApiResponse({status : 201, description: "Комната создана", type: String})
  @ApiResponse({status : 400, description: "Ошибка в параметрах", type: BadRequestSchema})
  @Post()
  async createRoom(@Body() body : SchemaRoom, @Res({ passthrough: true }) res: Response) {
    return await this.appService.createRoom(body);
  }

  @Delete(":roomId")
  async deleteRoom(@Param("roomId") roomId : string) {
    const result = await this.appService.deleteRoom(roomId)
    if (result.status != 200) {
      throw new HttpException("", result.status);
    }
    return
  }
}
