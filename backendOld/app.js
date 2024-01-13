import express from "express"
import path from "path"
import routeRooms from "./routes/room.js"
import routeAnime from "./routes/anime.js"
import routeAnimeColletion from "./routes/animeCollections.js"
import config from '../config/index.js';
import fs from 'fs'

const app = express()

app.use(express.json())

app.use("/animeResources/", express.static('../openingsLocal'));
app.use('/rooms/', routeRooms)
app.use('/anime/', routeAnime)
app.use('/animeCollection/', routeAnimeColletion)

app.get('/', (req, res) => {
    res.send(200)
})

app.listen(config.restPort, () => {
    console.log(`API запущено на порту ${config.restPort}`)
})

