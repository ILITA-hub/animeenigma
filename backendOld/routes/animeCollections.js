import { Router } from 'express'
import client from '../utils/caches.js'
import { generateRoomId } from '../utils/miscellaneous.js'
import pg from '../utils/pg.js'
import axios from 'axios'
import AnimeCollection from '../models/animeCollection.js'
import { validator, animeCollectionCreatePost } from '../middlewares/validation.js'

const router = Router()

router.get('/getAll', async (req, res) => {
    const result = await AnimeCollection.getAll()
    res.send(result)
})

router.post('/createAnimeCollection', 
    validator(animeCollectionCreatePost), 
    async (req, res) => {
        const body = req.body
        const idNewAnimeCollection = await AnimeCollection.createAnimeCollection(body.name, body.description, body.animeOpenings)
        res.sendStatus(200)
})

export default router