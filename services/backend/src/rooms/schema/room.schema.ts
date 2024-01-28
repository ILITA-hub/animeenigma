import { IsString, IsInt, IsNotEmpty, IsArray } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';
import { Type } from 'class-transformer';

export class SchemaRoom {
    @IsString()
    @IsNotEmpty()
    @ApiProperty()
    name: string;

    @IsString()
    @ApiProperty()
    description: string;

    // @IsString()
    // @IsNotEmpty()
    // @ApiProperty()
    // ownerId: string; // придумать как сделать

    @IsArray()
    @ApiProperty({ type: [Object] })
    rangeOpenings: Array<Object> = [{type : typeOpening.ALL, id: 0}];

    @IsInt()
    qtiUsersMax: number = 10;
}

enum typeOpening { ALL = "all", COLLECTION = "collection", GENRE = "genre"}