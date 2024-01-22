import { ApiProperty } from '@nestjs/swagger';

export class BadRequestSchema {

    @ApiProperty()
    message: String[]
    @ApiProperty()
    error: String
    @ApiProperty()
    statusCode: Number
}