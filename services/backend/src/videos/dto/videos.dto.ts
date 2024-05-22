import { IsNotEmpty } from 'class-validator';
import { ApiProperty } from '@nestjs/swagger';

export class VideosQueryDTO {
    @IsNotEmpty()
    @ApiProperty({
        default: 50,
        maximum: 50,
        minimum: 1
    })
    limit: number

    @IsNotEmpty()
    @ApiProperty({
        default: 1
    })
    page: number
}