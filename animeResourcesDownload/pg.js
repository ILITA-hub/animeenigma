import postgres from 'postgres';
import config from './config.tmpl.cjs';

const pg = postgres({
    host: config.pgHost,
    port: config.pgPort,
    username: config.pgUser,
    password: config.pgSecret,
    database: config.pgDB,
})

export default pg

// example 
// static async getById(id) {
//   return await pg`
//   SELECT * FROM public."openings" AS openings
//   WHERE openings."active" = true
//   AND openings."id" = ${id}
// `
// }
