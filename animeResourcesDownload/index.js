import { client } from 'node-shikimori';
import ytdl from 'ytdl-core';
import fs from 'fs'
import axios from 'axios'
import pg from './pg.js'
import randomUseragent from 'random-useragent';
import youtubedl  from 'youtube-dl-exec'

const shikimori = client({})
let animeArr = []
let animeOP = {}

function delay(time) {
    return new Promise(resolve => setTimeout(resolve, time));
}

// Цикл получения всех аниме
for(let i = 1; i <= 1; i++) {
    let countTry = 0
    let result 
    try {
        result = await axios.get(`https://shikimori.one/api/animes?page=${i}&limit=50&kind=tv&season=2020_2023&status=released&score=7`, {
            headers: {
                'User-Agent' : randomUseragent.getRandom()
            }
        })
    } catch {
        await new Promise(resolve => setTimeout(resolve, 1000))
        countTry++
        if (countTry > 10) continue
        i--
        continue
    }

    result = result['data']

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
        result = await axios.get(`https://shikimori.one/api/animes/${animeArr[i]}`, {
            headers: {
                'User-Agent' : randomUseragent.getRandom()
            }
        })
        result = result['data']
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
    console.log(`Пройдено ${i} аниме`)
}

for (let i = 0; i < Object.keys(animeOP).length; i++) {
    const key = Object.keys(animeOP)[i]
    const anime = animeOP[key]
    let result
    let count = 0
    try {
        if (count >= 5) {
            console.log(`Не смог получить инфу по ${anime['id']} аниме`)
            count = 0
            continue
        }
        result = await axios.get(`https://shikimori.one/api/animes/${key}/videos`, {
            headers: {
                'User-Agent' : randomUseragent.getRandom()
            }
        })
        console.log(`Получил инфу по ${anime['id']} аниме, ${i}`)

        result["data"].forEach(el2 => {
            if (el2.kind == 'op') {
                animeOP[key].op.push({
                    url: el2.url,
                    hosting: el2.hosting,
                    path: "",
                    name: el2.name
                })
            }
        })

        await delay(1000)
        count = 0
    } catch {
        console.log(`ы`)
        await delay(1000)
        i--
        count++
    }
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
                let outNameOp = `../animeResources/${animeOP[key].id}_${i+1}.mp4`
                animeOP[key].op[i].path = outNameOp
                await download(animeOP[key].op[i], outNameOp)
                // await ytdl(el.url, { filter: 'audioonly', quality: 'highestaudio' })
                //     .pipe(fs.createWriteStream(outNameOp))
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
            console.error('Ошибка при скачивании изображения:', URLIMAGE);
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
            await pg`INSERT INTO public."genresAnime"
            ("animeId", "genreId", active)
            VALUES(${anime.id}, ${anime.genres[i].id}, true)`
        }

        for(let i = 0; i < anime.op.length; i++) {
            let op = anime.op[i]
            await pg`INSERT INTO public.openings
            (id, active, "mp3OpPath", "animeId", "name")
            VALUES(nextval('openings_id_seq'::regclass), true, ${op.path}, ${anime.id}, ${op.name})`
        }
    }
}

console.log("Всё")

async function download(opening, outNameOp) {
    await new Promise(resolve => {
        const optionsYoutubedl = {
            format: 'best', 
            output: outNameOp, // Название выходного файла
        };
        youtubedl(opening['url'], {
            ...optionsYoutubedl
            // addHeader: ['referer:youtube.com', 'user-agent:googlebot']
        }).then(() => resolve())
    });
}