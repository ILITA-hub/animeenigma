import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common';
import { RoomService } from './room.service';
import { Room } from './dto/create-room.dto'

@Controller("rooms")
export class AppController {
  constructor(private readonly appService: RoomService) {}

  @Get("getAll")
  async getAllRooms() {
    return await this.appService.getAllRooms();
  }

  @Get(":roomId")
  async getRoom(@Param("roomId") roomId : string) {
    return await this.appService.getRoom(roomId);
  }

  @Post()
  async createRoom(@Body() body : Room, @Res({ passthrough: true }) res: Response) {
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
