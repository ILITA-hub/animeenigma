import { IsNotEmpty } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';
import { Socket } from 'socket.io';
import { Status } from './enum-status.dto'
import { UserEntity } from '../../users/entity/user.entity';

export class Room {
    id: string;

    name: string;

    description: string;

    status: Status = Status.START;

    openingId: number = -1;

    users: { [key: string] : UserEntity} = {};

    updatedAt: number;

    ownerId: string;

    historyAnime: Array<Number> = [];

    rangeOpenings: Array<Object> = [{type : "all", id : 0}];

    PORT: number

    constructor(id: string, name: string, ownerId: string, rangeOpenings: Array<Object>, PORT:number) {
        this.id = id
        this.name = name
        this.ownerId = ownerId
        if (rangeOpenings) {
            this.rangeOpenings = rangeOpenings
        }
        this.PORT = PORT
    }
}