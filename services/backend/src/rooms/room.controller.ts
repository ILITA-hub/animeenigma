import { Controller, Get, Post, Param, Body, Delete, Res, HttpException } from '@nestjs/common';
import { RoomService } from './room.service';
import { Room } from './dto/create-room.dto'
import { SchemaRoom } from './schema/room.schema'
import { BadRequestSchema } from './schema/badrequest.schema'
import { ApiBadRequestResponse, ApiBody, ApiCreatedResponse, ApiOperation, ApiResponse, ApiTags } from '@nestjs/swagger'

@ApiTags("Комнаты")
@Controller("rooms")
export class AppController {
  constructor(private readonly roomService: RoomService) {}

  @ApiOperation({ summary: "Получение всех комнат"})
  @Get()
  async getAllRooms() {
    return await this.roomService.getAllRooms();
  }

  // @ApiBody({ type: SchemaRoom })
  @ApiCreatedResponse({description: "Комната создана", type: String})
  @ApiBadRequestResponse({description: "Ошибка в параметрах", type: BadRequestSchema})
  @ApiOperation({ summary: "Создане комнаты"})
  @Post()
  async createRoom(@Body() body : SchemaRoom) {
    return await this.roomService.createRoom(body);
  }
}
