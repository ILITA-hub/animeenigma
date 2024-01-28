import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common';
import { RoomService } from './room.service';
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { BadRequestSchema } from './schema/badrequest.schema'
import { ApiBody, ApiResponse, ApiTags } from '@nestjs/swagger'

@ApiTags("Room")
@Controller("rooms")
export class AppController {
  constructor(private readonly roomService: RoomService) {}

  @Get("getAll")
  async getAllRooms() {
    return await this.roomService.getAllRooms();
  }

  @Get(":roomId")
  async getRoom(@Param("roomId") roomId : string) {
    const room = await this.roomService.getRoom(roomId);
    if (!room) {
      throw new HttpException("", 404);
    }
    return room
  }

  // @ApiBody({ type: SchemaRoom })
  @ApiResponse({status : 201, description: "Комната создана", type: String})
  @ApiResponse({status : 400, description: "Ошибка в параметрах", type: BadRequestSchema})
  @Post()
  async createRoom(@Body() body : SchemaRoom) {
    return await this.roomService.createRoom(body);
  }

  @Delete(":roomId")
  async deleteRoom(@Param("roomId") roomId : string) {
    // await this.roomService.deleteAll()
    await this.roomService.deleteRoom(roomId)
    return
  }
}
