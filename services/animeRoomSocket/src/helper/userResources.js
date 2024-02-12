import pg from './pg.js'

async function getUserByUsername (username) {
    return await pg`SELECT * FROM users WHERE name = '${username}'`
}

async function getUserByID (id) {
    const result = await pg`SELECT * FROM users WHERE "id" = ${id}`
    if (result.length > 0) {
        return result[0]
    }
    return null
}

export {getUserByUsername, getUserByID}