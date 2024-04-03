import { request, gql } from 'graphql-request'
import pg from './pg.js'

function createRequest(page) {
    return gql`   
    query Genres {
        genres(entryType: Anime) {
            name
            id
            kind
            russian
        }
    }
    `
}

async function init() {
    let result = await request('https://shikimori.one/api/graphql', createRequest())

    result = result["genres"].filter(n => {
        return n["kind"] == "genre"
    })

    for(let i = 0; i < result.length; i++) {
        const el = result[i]

        await pg`INSERT INTO public.genres (id, "name", "nameRu", active)
        VALUES(${el['id']}, ${el['name']}, ${el['russian']}, true)`
    }

    console.log("Всё")
}

init()