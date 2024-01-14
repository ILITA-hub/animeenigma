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

    @IsString()
    @IsNotEmpty()
    @ApiProperty()
    ownerId: string; // придумать как сделать

    @IsArray()
    @IsNotEmpty()
    @ApiProperty({ type: [Number] })
    rangeOpenings: number[];
}