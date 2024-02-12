import { getRandomInt } from '../helper/init.js'
import { getOP } from './getOp.js'
async function play(rangeOP, history) {
    let result = {
        id: -1,
        animeId: -1,
        animeNames: [],
        player: ""
    }
    console.log(history, 3)
    if (rangeOP.length == 0) {
        result = getOP(rangeOP[0], history)
    } else {
        result = getOP(rangeOP[getRandomInt(0,rangeOP.length-1)], history)
    }
    return result
}

export { play }