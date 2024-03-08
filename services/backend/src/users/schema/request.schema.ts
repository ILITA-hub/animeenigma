import { ApiProperty } from '@nestjs/swagger';

class BadRequestSchema401 {
    @ApiProperty()
    message: String

    @ApiProperty({default : 401})
    statusCode: Number
}

class SucsessfulRequest200 {
    @ApiProperty()
    token : String
}

export {BadRequestSchema401, SucsessfulRequest200}