import { request, gql } from 'graphql-request'
import pg from './pg.js'
import youtubedl from "youtube-dl-exec"
import path from 'path'
import * as Minio from 'minio'
import { unlink } from 'fs/promises';


async function getAnimeList() {
    let arrayAnimeResult = []
    for (let i = 1; i <= 50; i++) {
        let result = await request('https://shikimori.one/api/graphql', createRequest(i))
        arrayAnimeResult = [...arrayAnimeResult, ...result['animes']]
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
    for (let anime of animes) {
        if (anime['videos'].length <= 0) continue
        let animeNew = anime
        let openings = []
        for (let op of anime['videos']) {
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

    for (let anime of animes) {
        await pg`INSERT INTO public.anime
        (id, "name", "nameRU", "nameJP", "active", "year", "imgPath")
        VALUES(${anime['id']}, ${anime['name'] ? anime['name'] : anime['english']}, ${anime['russian']}, ${anime['japanese']}, true, ${anime["airedOn"]["year"]}, ${anime["poster"]["originalUrl"]})`

        for (let videos of anime['videos']) {
            try {
                const nameS3 = `${videos['id']}`
                let outNameOp = `../animeResources/${videos['name']}.mp4`
                await pg`INSERT INTO public.videos
                (id, "mp4Path", "name", "animeId", "active", "kind", "nameS3")
                VALUES(${videos['id']}, ${videos['playerUrl']}, ${videos['name'] ? videos['name'] : anime['name'] ? anime['name'] : anime['english']}, ${anime['id']}, true, ${videos['kind']}, ${nameS3});`
                await download(videos['playerUrl'], outNameOp, nameS3)
            } catch (e) {
                console.error(e)
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

async function download(opening, outNameOp, opName) {
    try {
        await youtubedl(opening, {
            cookies: "./cookies.txt",
            output: outNameOp,
            format: 'best',
            
        }).then(output => {
            console.log('Видео успешно загружено:', output);
        }).catch(err => {
            console.error('Произошла ошибка:', err);
        });

    } catch (error) {
        console.error('Ошибка при скачивании видео:', error);
    }


    const minioClient = new Minio.Client({
        endPoint: 'localhost',
        port: 9000,
        useSSL: false,
        accessKey: 'D3mQYLVKg1aJh7AJZQhH',
        secretKey: '0gc2EyEO5zBoiLSWzt073Eexfu6z5WXVJhtsZFND',
    })
    const bucket = 'openings'

    var metaData = {
        'Content-Type': 'video/mp4',
        'X-Amz-Meta-Testing': 1234,
        example: 5678,
    }

    await minioClient.fPutObject(bucket, opName, outNameOp, metaData, function (err, objInfo) {
        if (err) {
            return console.log(err)
        }
        console.log('Success', objInfo)
    })

    deleteFile(outNameOp)

    const presignedUrl = await minioClient.presignedUrl('GET', bucket, opName, 60 * 5)
}

start()

async function deleteFile(pathFile) {
    try {
      await unlink(pathFile);
      console.log('Файл успешно удален');
    } catch (err) {
      console.error('Ошибка при удалении файла:', err);
    }
  }