import { IsNotEmpty } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';
import { Socket } from 'socket.io';

export class AnimeCollectionDTO {
    @IsNotEmpty()
    @ApiProperty({
        type: String
    })
    name: string

    @IsNotEmpty()
    @ApiProperty({
        type: String
    })
    description: string

    @IsNotEmpty()
    @ApiProperty({
        type: [Number]
    })
    openings: number[]
}