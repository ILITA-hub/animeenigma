import { Router } from 'express'
import client from '../utils/caches.js'
import { generateRoomId } from '../utils/miscellaneous.js'
import { validator, roomsPost } from '../middlewares/validation.js'
import pg from '../utils/pg.js'
import axios from 'axios'

const router = Router()

router.get('/getAll', async (req, res) => {
    let result = await pg`SELECT 
    "id", "name", "nameRU", "nameJP", "imgPath",
    (SELECT json_agg(row_to_json(op)) FROM (select "id","mp3OpPath" from "openings" where "openings"."animeId" = "anime"."id") op) as openings
    FROM "anime"`
    res.send(result)
})

export default router