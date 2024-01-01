import express from "express"
import ws from "./webSocket.js"
import path from "path"

const app = express()
const port = 3000

app.get('/', (req, res) => {
    res.sendFile(path.resolve()+'/index.html')
})

app.get('/:roomId', (req, res) => {
    res.send({roomId: req.params.roomId})
})

app.listen(port, () => {
    console.log(`API запущено на порту ${port}`)
})

