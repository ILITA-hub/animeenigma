import {getAllOP, getRandomInt, getAnimeById, getAnimeNotById} from '../helper/init.js'

async function getOP(rangeOP, history) {
    let result = {
        id: -1,
        animeId: -1,
        animeNames: [],
        player: ""
    }
    console.log(history, 1)

    switch (rangeOP['type']) {
        case "all":
            let openings = await getAllOP()
            while (true) {
                const op = openings[getRandomInt(0,openings.length-1)]
                result.id = op['id']
                if (filterHistory(result.id,history)) {
                    const anime = await getAnimeById(op['animeId'])
                    result.animeId = op['animeId']
                    result.animeNames.push(anime['nameRU'])
                    result.player = op['mp4Path']
                    break
                }
            }
            break
    }

    result.animeNames.push(getAnimeName(result.animeId))
    result.animeNames.push(getAnimeName(result.animeId))
    result.animeNames.push(getAnimeName(result.animeId))

    return result
}

async function filterHistory(id, history) {
    console.log(history, 2)
    if (history.indexOf(id) >= 0) {
        return false
    } else {
        return true
    }
}

async function getAnimeName(animeId) {
    const animes = await getAnimeNotById(animeId)
    return animes[getRandomInt(0,animes.length-1)]['nameRU']
}

export { getOP }