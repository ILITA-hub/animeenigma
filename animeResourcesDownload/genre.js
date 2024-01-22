import { client } from 'node-shikimori'
import pg from './pg.js'
import axios from 'axios'

const shikimori = client({})

let result
let good = false
const URL = "https://shikimori.one/api/genres"

while (!good) {
    try {
        result = await axios.get(URL)
        good = true
    } catch (e) {
        await new Promise(resolve => setTimeout(resolve, 1000))
        console.log(`ыыыыааааа`, e)
    }
}

for (let i = 0; i < result.data.length; i++) {
    let el = result.data[i]
    if (el.entry_type != "Anime") continue
    // console.log(`INSERT INTO public.genres (id, active, "createdAt", "updatedAt", "name", "nameRU") VALUES(${el.id}, true, now(), now(), '${el.name}', '${el.russian}')`)
    await pg`INSERT INTO
        public."genres"
        (id, active, "name", "nameRu")
        VALUES(${el.id}, true, ${el.name}, ${el.russian})
    `
}
console.log("Всё")