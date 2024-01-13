import postgres from 'postgres';
import config from '../../config/index.js';

const pg = postgres({
    host: config.pgHost,
    port: config.pgPort,
    username: config.pgUser,
    password: config.pgSecret,
    database: config.pgDB,
})

async function initDB() {
    await pg`
    CREATE TABLE IF NOT EXISTS public."openings" (
      "id" SERIAL PRIMARY KEY,
      "active" BOOLEAN NOT NULL DEFAULT true,
      "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "mp3OpPath" VARCHAR(255) NOT NULL,
      "animeId" INTEGER NOT NULL
    );
  `;

    await pg`
    CREATE TABLE IF NOT EXISTS public."anime" (
      "id" SERIAL PRIMARY KEY,
      "active" BOOLEAN NOT NULL DEFAULT true,
      "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "name" VARCHAR(255) NOT NULL,
      "nameRU" VARCHAR(255) NOT NULL,
      "nameJP" VARCHAR(255) NOT NULL,
      "description" TEXT NOT NULL,
      "imgPath" VARCHAR(255) NOT NULL
    );
  `;

    await pg`
    CREATE TABLE IF NOT EXISTS public."genres" (
      "id" SERIAL PRIMARY KEY,
      "active" BOOLEAN NOT NULL DEFAULT true,
      "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "name" VARCHAR(255) NOT NULL,
      "nameRU" VARCHAR(255) NOT NULL
    );
  `;

    await pg`
    CREATE TABLE IF NOT EXISTS public."animeGenres" (
      "animeId" INTEGER NOT NULL,
      "genreId" INTEGER NOT NULL,
      "active" BOOLEAN NOT NULL DEFAULT true,
      "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      PRIMARY KEY ("animeId", "genreId")
    );
  `;

    await pg`
    CREATE TABLE IF NOT EXISTS public."animeCollectionOpenings" (
      "idAnimeCollection" INTEGER NOT NULL,
      "idAnimeOpening" INTEGER NOT NULL,
      "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      PRIMARY KEY ("idAnimeCollection", "idAnimeOpening")
    );
  `;

    await pg`
    CREATE TABLE IF NOT EXISTS public."animeCollection" (
      "id" SERIAL PRIMARY KEY,
      "name" VARCHAR(255) NOT NULL,
      "description" VARCHAR(255) NOT NULL DEFAULT true,
      "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
      "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW()
    );
  `;

}

await initDB();

export default pg

// example 
// static async getById(id) {
//   return await pg`
//   SELECT * FROM public."openings" AS openings
//   WHERE openings."active" = true
//   AND openings."id" = ${id}
// `
// }
