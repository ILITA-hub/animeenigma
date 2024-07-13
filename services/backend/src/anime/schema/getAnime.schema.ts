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
        description: "Жанры аниме",
        required: false,
    })
    genres: []

    @ApiProperty({
        description: "Год выпуска аниме",
        required: false,
        example: ["2024", "2023"]
    })
    year: String[]
}

class VideosArrayResponse {
    @ApiProperty({
        example: 0
    })
    id: number

    @ApiProperty({
        example: "//youtube.com/embed/QoGM9hCxr4k"
    })
    mp4Path: string

    @ApiProperty({
        example: "OP1 «Yuusha» — YOASOBI"
    })
    name: string

    @ApiProperty({
        example: "op"
    })
    kind: string
}

class Genre {
    @ApiProperty({
        example: 0
    })
    id: number

    @ApiProperty({
        example: "Drama"
    })
    name: string

    @ApiProperty({
        example: "Драма"
    })
    nameRU: string
}

class GenresArrayResponse {
    @ApiProperty({
        example: 0
    })
    id: number

    @ApiProperty()
    genre: Genre
}

export class GetAnimeResponse {
    @ApiProperty({
        example: 0
    })
    id: number

    @ApiProperty({
        example: "Sousou no Frieren"
    })
    name: string

    @ApiProperty({
        example: "Провожающая в последний путь Фрирен"
    })
    nameRU: string

    @ApiProperty({
        example: "葬送のフリーレン"
    })
    nameJP: string

    @ApiProperty({
        example: 2024
    })
    year: number

    @ApiProperty({
        isArray: true
    })
    videos: VideosArrayResponse

    @ApiProperty({
        isArray: true
    })
    genres: GenresArrayResponse
}