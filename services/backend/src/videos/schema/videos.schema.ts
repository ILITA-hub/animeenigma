import { ApiProperty } from '@nestjs/swagger';

export class VideoSchemaById200 {
    @ApiProperty()
    id: Number

    @ApiProperty()
    mp4Path: String

    @ApiProperty()
    name: String

    @ApiProperty()
    kind: String
}

export class VideoSchemaById404 {
    @ApiProperty({
        default: 404
    })
    statusCode: Number

    @ApiProperty({
        default: "Видео не найдено"
    })
    message: String
}

export class VideoSchemaByAnime200 {
    @ApiProperty()
    id: Number

    @ApiProperty()
    mp4Path: String

    @ApiProperty()
    name: String

    @ApiProperty()
    kind: String
}

export class VideoSchemaByAnime404 {
    @ApiProperty({
        default: 404
    })
    statusCode: Number

    @ApiProperty({
        default: "Аниме не найдено"
    })
    message: String
}