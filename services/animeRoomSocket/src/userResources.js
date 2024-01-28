import pg from './pg.js'

async function getUserByUsername (username) {
    return await pg`SELECT * FROM users WHERE name = '${username}'`
}

async function getUserByID (id) {
    return await pg`SELECT * FROM users WHERE "id" = ${id}`
}

export {getUserByUsername, getUserByID}