import pg from './pg.js'

async function getOPById() {
    return await pg`SELECT * from videos Where "id" = ${id}`
}

async function getAllOP() {
    return await pg`Select * from videos`
}

async function getOPNotById(id) {
    return await pg`SELECT * from videos Where "id" <> ${id}`
}

async function getAnimeById(id) {
    return await pg`SELECT * from anime Where "id" = ${id}`
}

async function getAnimeNotById(id) {
    return await pg`SELECT * from anime Where "id" <> ${id}`
}

export {getOPById, getAllOP, getOPNotById, getAnimeById, getAnimeNotById}