import express from "express";
import bodyParser from "body-parser";
import { WebSocketServer } from "ws";
import { uuidv4 } from './helpers/token.js'
import { getCache } from './helpers/redis.js'
import pg from './helpers/pg.js'
import { getPlayingOpenimg, shuffle } from './helpers/playing.js'
import * as Minio from 'minio'

const minioClient = new Minio.Client({
    endPoint: 'localhost',
    port: 9000,
    useSSL: false,
    accessKey: 'D3mQYLVKg1aJh7AJZQhH',
    secretKey: '0gc2EyEO5zBoiLSWzt073Eexfu6z5WXVJhtsZFND',
})
const bucket = 'openings'

const app = express()
app.use(bodyParser.json())
const wss = new WebSocketServer({ port: 1234 })
let rooms = {
    "example": {
        users: [
            {
                id: "id",
                ready: false,
                nickName: "username",
                score: 1,
                ws: "ws",
                load: false
            }
        ],
        opening: {
            url: "",
            id: 1,
            name: "name"
        },
        timeout: null,
        chat: [
            {
                nickName: "nickName",
                message: "message"
            }
        ],
        openings: [],
        history: [],
        status: "wait",
        owner: 1
    }
}

const clients = new Map()

app.post("/create_room", async (req, res) => {
    const body = req.body
    const idRoom = body.uniqueURL
    const entity = body.openingsType
    const maxPlayer = body.maxPlayer
    const status = body.status
    const name = body.name
    const openings = body.openings

    const room = await pg`INSERT INTO room
        ("name", "maxPlayer", status, "uniqueURL", "createdAt", "updatedAt")
        VALUES(${name}, ${maxPlayer}, ${status}, ${idRoom}, now(), now())
        returning id, name`

    await entity.forEach(async value => {
        if (value['type'] && value['id']) {
            await pg`INSERT INTO "roomOpenings"
                ("idRoom", "type", "idEntity")
                VALUES
                (${room[0].id}, ${value['type']}, ${value['id']})`
        } else {
            console.log('Invalid value for type or idEntity:', value);
        }
    })

    rooms[idRoom] = {
        users: [],
        openings: openings,
        timeout: null,
        chat: [],
        opening: {
            url: "",
            id: -1,
            name: ""
        },
        history: [],
        status: "wait",
        owner: 1
    }

    console.log(rooms[idRoom])

    res.sendStatus(200)
})

app.get("/rooms", async (req, res) => {
    res.send(rooms)
})

app.post("/stop_room", async (req, res) => {
    const body = req.body
    rooms[body.roomsId]['status'] = "wait"
    console.log(rooms)

    res.sendStatus(200)
})

wss.on("connection", async (ws, req) => {
    ws.on("error", console.error)

    ws.on("message", data => {
        const messageBody = JSON.parse(data)
        const roomId = req.url.split('/')[1]
        const token = req.url.split('/')[2]

        switch (messageBody["type"]) {
            // case "newQuestion":
            //     playVideo("631b7", "//youtube.com/embed/j3p6sXq_uUM")
            //     break
            case "userIsReady":
                if (!rooms[roomId]) {
                    ws.close(1000, JSON.stringify({
                        message: "Комната не найдена"
                    }))
                }

                readyPlayer(roomId, messageBody['clientId'])
                checkedReadyRoom(roomId)
                // runGame(roomId)
                break
            case "openingIsLoaded":
                openingIsLoaded(roomId, clientId)
                break
            case "checkAnswer":
                checkAnswer(clientId, messageBody['answer'], roomId)
                break
        }
    })

    const roomId = req.url.split('/')[1]
    const token = req.url.split('/')[2]
    const clientId = `${uuidv4()}_${roomId}`
    const userSession = await getCache(`userSession${token}`)
    const userIdDB = userSession['userId']
    const user = await pg`SELECT username FROM users WHERE id = ${userIdDB}`
    if (rooms[roomId]) {
        rooms[roomId]['users'].push(
            {
                id: clientId,
                ready: false,
                nickName: user[0]['username'],
                score: 1,
                ws: ws,
                load: false
            }
        )

        clients.set(clientId, ws)

        ws.send(JSON.stringify({
            type: "connect",
            clientId: clientId,
            status: rooms[roomId]['status']
        }))

        rooms[roomId]['users'].forEach(value => {
            value['ws'].send(
                JSON.stringify({
                    type: "updUsers",
                    users: rooms[roomId]['users']
                })
            )
        })
    } else {
        ws.close(1000, JSON.stringify({
            message: "Комната не найдена"
        }))
    }

    ws.on("close", () => {
        clients.delete(clientId)
        deleteUsers(clientId)
    })
})


function deleteUsers(id) {
    for (let room in rooms) {
        for (let i = 0; i < rooms[room]["users"].length; i++) {
            if (rooms[room]["users"][i]["id"] == id) {
                rooms[room]["users"].splice(i, 1)
                break
            }
        }
    }
}

function playVideo(idRoom, opening) {
    rooms[idRoom].users.forEach((value, key) => {
        value.send(JSON.stringify({
            type: "newQuestion",
            opening: opening
        }))
    })
}

app.listen(1000)

function readyPlayer(roomId, clientId) {
    for (let i = 0; i < rooms[roomId]["users"].length; i++) {
        if (rooms[roomId]['users'][i]['id'] == clientId) {
            rooms[roomId]['users'][i]['ready'] = true
        }
    }
}

function checkedReadyRoom(roomId) {
    if (rooms[roomId]['status'] == 'playing') {
        return
    }

    const readyPlayerCount = rooms[roomId]['users'].filter(user => user['ready']).length
    const playerCount = rooms[roomId]['users'].length

    const ready = (readyPlayerCount / playerCount) * 100

    if (ready >= 100 && rooms[roomId]['status'] != 'playing') {
        rooms[roomId]['status'] = "playing"
        newOpening(roomId)
    }
}

async function newOpening(roomId) {
    if (rooms[roomId]['status'] == 'wait') {
        return
    }

    const openingId = getPlayingOpenimg(rooms[roomId]['openings'], rooms[roomId]['history'])

    let url = `http://46.181.201.172:9000/openings/${openingId}`

    const anime = (await pg`SELECT anime."id" as animeId, anime."nameRU", videos.id as videoId
        from videos join anime ON anime.id = videos."animeId" where videos.id = ${openingId}`)[0]
    const animes = Array.from(await pg`SELECT anime.id as animeid, anime."nameRU" FROM anime where anime.id != ${anime['animeid']} ORDER BY random() LIMIT 3`)

    let openings = shuffle([anime, ...animes])

    rooms[roomId]['users'].forEach((value, key) => {
        if (!value['ready']) {
            return
        }

        value['ws'].send(JSON.stringify({
            type: "newOpening",
            opening: url,
            answers: [
                {
                    id: openings[0]['animeid'],
                    name: openings[0]['nameRU']
                },
                {
                    id: openings[1]['animeid'],
                    name: openings[1]['nameRU']
                },
                {
                    id: openings[2]['animeid'],
                    name: openings[2]['nameRU']
                },
                {
                    id: openings[3]['animeid'],
                    name: openings[3]['nameRU']
                }
            ]
        }))
    })

    rooms[roomId]['opening'] = {
        url: url,
        id: anime['animeid'],
        name: anime['nameRU']
    }

    rooms[roomId]['history'].push(openingId)

    if (rooms[roomId]['history'].length >= 5) {
        rooms[roomId]['history'].shift()
    }

    console.log("-----------------------")
    console.log(rooms[roomId]['opening'])
}

async function openingIsLoaded(roomId, clientId) {
    console.log(roomId, clientId, 'loaded')

    for (let i = 0; i < rooms[roomId]["users"].length; i++) {
        if (rooms[roomId]['users'][i]['id'] == clientId) {
            rooms[roomId]['users'][i]['load'] = true
        }
    }

    const loadPlayerCount = rooms[roomId]['users'].filter(user => user['load']).length
    const readyPlayerCount = rooms[roomId]['users'].filter(user => user['ready']).length

    if (loadPlayerCount >= readyPlayerCount) {
        runGame(roomId)
    }
}

async function runGame(roomId) {
    if (rooms[roomId]['status'] == 'wait') {
        return
    }

    rooms[roomId]['users'].forEach((value, key) => {
        if (!value['ready']) {
            return
        }

        value["ws"].send(JSON.stringify({
            type: "startOpening"
        }))
    })

    setTimeout(() => {
        rooms[roomId]['users'].forEach((value, key) => {
            value['ws'].send(JSON.stringify({
                type: "endOpening",
                trueAnswer: {
                    id: rooms[roomId]['opening']['id'],
                    name: rooms[roomId]['opening']['name']
                }
            }))
        })

        loadIsFalse(roomId)

        setTimeout(newOpening, 5000, roomId)
    }, 10000)
}

async function checkAnswer(userId, answer, roomId) {
    if (rooms[roomId]['opening']['id'] == answer) {
        rooms[roomId]['users'].find(user => user.id == userId).score += 1
        rooms[roomId]['users'].forEach(value => {
            value['ws'].send(
                JSON.stringify({
                    type: "updUsers",
                    users: rooms[roomId]['users']
                })
            )
        })
    }
}

function loadIsFalse(roomId) {
    for (let i = 0; i < rooms[roomId]["users"].length; i++) {
        rooms[roomId]['users'][i]['load'] = false
    }
}