import { IsNotEmpty } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';
import { Socket } from 'socket.io';
import { Status } from './enum-status.dto'

export class Room {
    id: string;

    name: string;

    description: string;

    status: Status;

    openingId: string;

    users: Array<Socket>;

    updatedAt: number;

    ownerId: string;

    historyAnime: Array<Number>;

    rangeOpenings: Array<Number>;

    constructor(id: string, name: string, ownerId: string, rangeOpenings: Array<Number>) {
        this.id = id
        this.name = name
        this.ownerId = ownerId
        this.rangeOpenings = rangeOpenings
    }
}