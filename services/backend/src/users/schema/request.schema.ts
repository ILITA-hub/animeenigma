import { ApiProperty } from '@nestjs/swagger';

class BadRequestSchema400 {
    @ApiProperty()
    message: String

    @ApiProperty({default : 400})
    statusCode: Number
}

class SucsessfulRequest200 {
    @ApiProperty()
    token : String
}

export {BadRequestSchema400, SucsessfulRequest200}