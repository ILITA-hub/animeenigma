import { client } from 'node-shikimori';
import ytdl from 'ytdl-core';
import fs from 'fs'
import axios from 'axios'
import pg from '../backend/utils/pg.js'

const shikimori = client({})
let animeArr = []
let animeOP = {}

// Цикл получения всех аниме
for(let i = 1; i <= 1; i++) {
    let countTry = 0
    let result
    try {
        result = await shikimori.animes.list({
            kind: 'tv',
            limit: 50,
            page: i
        })
    } catch {
        await new Promise(resolve => setTimeout(resolve, 1000))
        countTry++
        if (countTry > 10) continue
        i--
        continue
    }
    
    if (result.length == 0) break

    result.forEach(el => {
        animeArr.push(el.id)
    })
    console.log(`Пройдено ${i} страниц аниме`)
}

// цикл получения ссылок на опенинги
for(let i = 0; i < animeArr.length; i++) {
    let countTry = 0
    let result
    try {
        result = await shikimori.animes.byId({
            id: animeArr[i]
        })
    } catch {
        await new Promise(resolve => setTimeout(resolve, 1000))
        countTry++
        if (countTry > 10) continue
        i--
        continue
    }

    if (result.videos.length == 0) {
        continue
    }
    animeOP[result.id] = {
        id: result.id,
        en_name: result.name,
        ru_name: result.russian,
        jp_name: result.japanese[0],
        op: [],
        img: {
            url: result.image.original,
            path: ""
        },
        description: (result.description != null) ? result.description : "",
        genres: result.genres
    }

    result["videos"].forEach(el2 => {
        if (el2.kind == 'op') {
            animeOP[result.id].op.push({
                url: el2.url,
                hosting: el2.hosting,
                path: ""
            })
        }
    })
    console.log(`Пройдено ${i} аниме`)
}

//цикл проверки аниме на наличие
for(let key in animeOP) {
    const result = await pg`SELECT id FROM public.anime WHERE id = ${key}`
    if (result.length != 0) animeOP[key] = undefined
}

for(let key in animeOP) {
    let anime = animeOP[key]
    if (anime != undefined) {
        let countOp = animeOP[key].op.length
        if (countOp == 0) animeOP[key] = undefined
    }
}

// цикл скачивания опенингов
for(let key in animeOP) {
    if (animeOP[key] != undefined) {

        for(let i = 0; i < animeOP[key].op.length; i++) {
            let el = animeOP[key].op[i]
            if (el.hosting == "youtube") {
                let outNameOp = `../animeResources/${animeOP[key].id}_${i+1}.mp3`
                animeOP[key].op[i].path = outNameOp
                await ytdl(el.url, { filter: 'audioonly', quality: 'highestaudio' })
                    .pipe(fs.createWriteStream(outNameOp))
            }
        }

    }
}

// цикл скачивания картинок
for(let key in animeOP) {
    if (animeOP[key] != undefined) {

        let outNameOp = `../animeResources/${animeOP[key].id}.jpg`
        const URLIMAGE = `https://shikimori.one${animeOP[key].img.url}`;
        await axios({
            method: 'get',
            url: URLIMAGE,
            responseType: 'stream'
        })
        .then(function (response) {
            response.data.pipe(fs.createWriteStream(outNameOp))
            .on('close', () => console.log(`Изображение аниме ${animeOP[key].en_name} успешно скачано`));
            animeOP[key].img.path = outNameOp
        })
        .catch(function (error) {
            console.error('Ошибка при скачивании изображения:', error);
        });

    }
}

for(let key in animeOP) {
    let anime = animeOP[key]
    if (anime != undefined) {

        await pg`INSERT INTO public.anime
        (id, active, "name", "nameRU", "nameJP", description, "imgPath")
        VALUES(${anime.id}, true, ${anime.en_name}, ${anime.ru_name}, ${anime.jp_name}, ${anime.description}, ${anime.img.path})`

        for(let i = 0; i < anime.genres.length; i++) {
            await pg`INSERT INTO public."animeGenres"
            ("animeId", "genreId", active)
            VALUES(${anime.id}, ${anime.genres[i].id}, true)`
        }

        for(let i = 0; i < anime.op.length; i++) {
            let op = anime.op[i]
            await pg`INSERT INTO public.openings
            (id, active, "mp3OpPath", "animeId")
            VALUES(nextval('openings_id_seq'::regclass), true, ${op.path}, ${anime.id})`
        }
    }
}