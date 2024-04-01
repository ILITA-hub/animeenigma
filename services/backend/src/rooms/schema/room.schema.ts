import { IsString, IsInt, IsNotEmpty, IsArray } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';
import { Type } from 'class-transformer';

export class SchemaRoom {
    @IsString()
    @IsNotEmpty()
    @ApiProperty({
        description: "Название комнаты",
        example: "МЕГА КРУТЫЕ АНИМЕ"
    })
    name: string;

    @IsArray()
    @ApiProperty({ 
        type: [Object],
        description: "Опенинги",
        example: [
            {type: "all", id: 0},
            {type: "collection", id: 1},
            {type: "anime", id: 1}
        ]
    })
    rangeOpenings: Array<rangeOp> = [{type : "all", id: 0}];

    @IsInt()
    @ApiProperty({
        description: "Максимальное колличество игроков",
        example: 10
    })
    qtiUsersMax: number = 10;
}

interface rangeOp {
    type: string
    id: number
}