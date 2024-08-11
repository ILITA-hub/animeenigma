import { request, gql } from 'graphql-request'
import pg from './pg.js'
import youtubedl from "youtube-dl-exec"

async function getAnimeList() {
    let arrayAnimeResult = []
    for(let i = 1; i <= 50; i++) {
        let result = await request('https://shikimori.one/api/graphql', createRequest(i))
        arrayAnimeResult = [...arrayAnimeResult,...result['animes']]
        console.log(`Прошла ${i} страница`)
        await new Promise((res) => {
            setTimeout(() => {
                res()
            }, 500)
        })
    }
    console.log(arrayAnimeResult.length, `GET`)
    return arrayAnimeResult
}

function createRequest(page) {
    return gql`
    query Animes {
        animes(limit: 50, page: ${page}, kind: "tv", score: 7) {
            name
            english
            id
            japanese
            russian
            videos {
                id
                playerUrl
                name
                kind
            }
            genres {
                id
                name
                russian
            }
            airedOn {
                year
            }
            poster {
                originalUrl
            }
        }
    }    
`
}

async function filterAnimeOP(animes) {
    let arrayAnime = []
    for(let anime of animes) {
        if (anime['videos'].length <= 0) continue
        let animeNew = anime
        let openings = []
        for(let op of anime['videos']) {
            let regexp = /youtube.com/i;
            if ((op['kind'] == "op" || op['kind'] == "ed") && regexp.test(op['playerUrl'])) {
                openings.push(op)
            }
        }
        if (openings.length <= 0) continue
        animeNew['videos'] = openings
        arrayAnime.push(animeNew)
    }
    console.log(arrayAnime.length, "Прошла фильтрация")
    return arrayAnime
}

async function addAnimeInDB(animes) {

    for(let anime of animes) {
        await pg`INSERT INTO public.anime
        (id, "name", "nameRU", "nameJP", "active", "year", "imgPath")
        VALUES(${anime['id']}, ${anime['name'] ? anime['name'] : anime['english']}, ${anime['russian']}, ${anime['japanese']}, true, ${anime["airedOn"]["year"]}, ${anime["poster"]["originalUrl"]})`

        for(let videos of anime['videos']) {
            try {
                let outNameOp = `../animeResources/${videos['name']}.mp4`
                await pg`INSERT INTO public.videos
                (id, "mp4Path", "name", "animeId", "active", "kind")
                VALUES(${videos['id']}, ${videos['playerUrl']}, ${videos['name'] ? videos['name'] : anime['name'] ? anime['name'] : anime['english']}, ${anime['id']}, true, ${videos['kind']});`
                // await download(videos['playerUrl'], outNameOp)
            } catch (e) {
                console.error(e)
                // console.log(videos)
            }
        }

        for(let genre of anime['genres']) {
            await pg`INSERT INTO public."genresAnime"
            ("animeId", "genreId", "active")
            VALUES(${anime['id']}, ${genre['id']}, true);`
        }
    }

}

async function start() {
    let anime = await getAnimeList()
    anime = await filterAnimeOP(anime)
    await addAnimeInDB(anime)
    console.log('Всё')
}

async function download(opening, outNameOp) {
    console.log(`пошла закачка ${opening} в ${outNameOp}`)
    await new Promise((resolve, rej) => {
        const optionsYoutubedl = {
            format: 'best', 
            output: outNameOp, // Название выходного файла
        };
        youtubedl(opening, {
            ...optionsYoutubedl
            // addHeader: ['referer:youtube.com', 'user-agent:googlebot']
        }).then(output => {
            console.log(output)
            resolve()
        })
    });
}

start()