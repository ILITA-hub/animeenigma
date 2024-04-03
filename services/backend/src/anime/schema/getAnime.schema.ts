import { ApiProperty } from '@nestjs/swagger';

export class GetAnimeRequest {
    @ApiProperty({
        default: 50,
        description: "Колличество аниме на странице",
        maximum: 50,
        minimum: 1
    })
    limit: number

    @ApiProperty({
        default: 1,
        description: "Страница аниме"
    })
    page: number

    @ApiProperty({
        description: "Жанры аниме"
    })
    genres: []
}