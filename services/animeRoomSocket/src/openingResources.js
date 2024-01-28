import pg from './pg.js'

async function getOpAll() {
    return await pg`SELECT * FROM openings WHERE "mp3OpPath" <> ''`
}

async function getAnime(op) {
    let result = {
        nameOP: "",
        nameAnime: []
    }

    result.nameOP = await pg`SELECT nameRU FROM anime WHERE "id" == ${op.animeId}`[0]
    result.nameAnime = await pg`SELECT nameRU FROM anime WHERE "id" <> ${op.animeId}`

    return result
}

export { getOpAll, getAnime }