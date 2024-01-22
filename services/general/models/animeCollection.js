import pg from '../utils/pg.js'

class AnimeCollection {
    static async createAnimeCollection(name, description, animeOpenings) {
        let result = await pg`INSERT INTO "animeCollection" ("name", "description") VALUES (${name},${description}) RETURNING "id"`
        for(let i = 0; i < animeOpenings.length; i++) {
            await pg`INSERT INTO "animeCollectionOpenings" ("idAnimeCollection", "idAnimeOpening") VALUES (${result[0].id},${animeOpenings[i]})`
        }
        return result
    }

    static async getAll() {
        let result = await pg`SELECT "id", "name", "description" FROM "animeCollection"`
        return result
    }
}

export default AnimeCollection