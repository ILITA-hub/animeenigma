import { IsNotEmpty } from 'class-validator';

export class Room {
    id: string;

    @IsNotEmpty()
    name: string;

    description: string;

    status: string;

    openingId: string;

    users: object;

    updatedAt: number;

    @IsNotEmpty()
    ownerId: number;

    historyAnime: Array<number>;

    @IsNotEmpty()
    rangeAnime: Array<number>;

    constructor(id: string, name:string, ownerId: number, rangeAnime: Array<number>) {
        this.id = id
        this.name = name
        this.ownerId = ownerId
        this.rangeAnime = rangeAnime
    }
}

// const Status = { START: 'start', PLAYING: 'playing', BREAK: 'break' };