import express from "express"
import ws from "./webSocket.js"
import path from "path"
import routeRooms from "./routes/room.js"
import config from '../config/index.js';
import fs from 'fs'

const app = express()

app.use(express.json())

app.use("/opening/", express.static('../openingsLocal'));
app.use('/rooms/', routeRooms)

app.get('/', (req, res) => {
    
})

app.listen(config.restPort, () => {
    console.log(`API запущено на порту ${config.restPort}`)
})

