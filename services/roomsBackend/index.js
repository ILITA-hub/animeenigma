import express from "express";
import bodyParser from "body-parser";
import { WebSocketServer } from "ws";
import { uuidv4 } from './helpers/token.js'
import { getCache } from './helpers/redis.js'
import pg from './helpers/pg.js'
import { getPlayingOpemimg, shuffle } from './helpers/playing.js'

const app = express()
app.use(bodyParser.json())
const opening = "http://46.181.201.172:9001/api/v1/download-shared-object/aHR0cDovLzEyNy4wLjAuMTo5MDAwL29wZW5pbmdzL1kybWF0ZS5teC0lRTMlODAlOEMlRTMlODElOEIlRTMlODElOEYlRTMlODIlOTklRTMlODIlODQlRTYlQTclOTglRTMlODElQUYlRTUlOTElOEElRTMlODIlODklRTMlODElOUIlRTMlODElOUYlRTMlODElODQtJUUzJTgyJUE2JUUzJTgzJUFCJUUzJTgzJTg4JUUzJTgzJUE5JUUzJTgzJUFEJUUzJTgzJTlFJUUzJTgzJUIzJUUzJTgzJTg2JUUzJTgyJUEzJUUzJTgzJTgzJUUzJTgyJUFGLSVFMyU4MCU4RCVFMyU4MiVBQSVFMyU4MyVCQyVFMyU4MyU5NSVFMyU4MiU5QSVFMyU4MyU4QiVFMyU4MyVCMyVFMyU4MiVBRiVFMyU4MiU5OSVFNiU5OCVBMCVFNSU4MyU4RiUyMCVFMiU5OSVBQSVFOSU4OCVCNCVFNiU5QyVBOCVFOSU5QiU4NSVFNCVCOSU4QiUyMGZlYXQuJTIwJUUzJTgxJTk5JUUzJTgxJTg1JUUzJTgwJThDR0lSSSUyMEdJUkklRTMlODAlOEQubXA0P1gtQW16LUFsZ29yaXRobT1BV1M0LUhNQUMtU0hBMjU2JlgtQW16LUNyZWRlbnRpYWw9UlBNTTVMNVU5M0gxNkRHTktWMDUlMkYyMDI0MDkwMSUyRnVzLWVhc3QtMSUyRnMzJTJGYXdzNF9yZXF1ZXN0JlgtQW16LURhdGU9MjAyNDA5MDFUMTIzMDU5WiZYLUFtei1FeHBpcmVzPTQzMjAwJlgtQW16LVNlY3VyaXR5LVRva2VuPWV5SmhiR2NpT2lKSVV6VXhNaUlzSW5SNWNDSTZJa3BYVkNKOS5leUpoWTJObGMzTkxaWGtpT2lKU1VFMU5OVXcxVlRrelNERTJSRWRPUzFZd05TSXNJbVY0Y0NJNk1UY3lOVEl6TnpBMU5pd2ljR0Z5Wlc1MElqb2lVazlQVkU1QlRVVWlmUS5Ud0NTY1R2MzZ6bV9xY1lGR24tamNJdTBFNjlUUGZ2NTRxTVRXTTBuRHR4anA2N3cxWDBNQ1hpZjY4RkFScDEwanB5V1hMZjQxejBrdnF3ZXRTVndNUSZYLUFtei1TaWduZWRIZWFkZXJzPWhvc3QmdmVyc2lvbklkPW51bGwmWC1BbXotU2lnbmF0dXJlPTk4ZjNiOTVkZTJjY2JmMGE0ZmM3OTJlMDk3YTk3YTY4MDIyMTM5ZTY5OTc2YmM0NDA3ZWM5N2QyNzk3MjJhNzg"
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
    }
}

const clients = new Map()

app.post("/create_room", async (req, res) => {
    const body = req.body
    const idRoom = await pg`SELECT * FROM room WHERE "uniqueURL" = ${body.roomsId}`
    const openings = []
    const openingsDBTypes = await pg`SELECT * FROM "roomOpenings" WHERE "idRoom" = ${idRoom[0]['id']}`
    const openingsTypes = Array.from(openingsDBTypes)

    for(let type of openingsTypes) {
        switch (type['type']) {
            case "collection":
                const opening = await pg`SELECT * FROM "animeCollectionOpenings" WHERE "animeCollectionId" = ${type['idEntity']}`
                Array.from(opening).forEach(value => {
                    openings.push(value['animeOpeningId'])
                })
                break
            case "anime":
                break
        }
    }

    rooms[body.roomsId] = {
        users: [],
        openings: openings,
        timeout: null,
        chat: [],
        opening: {
            url: opening,
            id: 1,
            name: "name"
        },
        history: [],
        status: "wait"
    }

    console.log(rooms)

    res.sendStatus(200)
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
        // console.log(messageBody)
        const roomId = req.url.split('/')[1]
        const token = req.url.split('/')[2]

        switch (messageBody["type"]) {
            case "newQuestion":
                playVideo("631b7", "//youtube.com/embed/j3p6sXq_uUM")
                break
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
        }
    })

    const roomId = req.url.split('/')[1]
    const token = req.url.split('/')[2]
    const clientId = `${uuidv4()}_${roomId}`
    const userSession = await getCache(`userSession${token}`)
    console.log(userSession)
    console.log(token)
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
        // console.log(rooms[roomId])
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
        // console.log(rooms)
    })
})


function deleteUsers(id) {
    for (let room in rooms) {
        for (let i = 0; i < rooms[room]["users"].length; i++) {
            console.log(rooms[room]["users"][i]["id"], id)
            if (rooms[room]["users"][i]["id"] == id) {
                rooms[room]["users"].splice(i, 1)
                console.log(rooms[room]["users"])
                break
            }
        }
    }
}

function playVideo(idRoom, opening) {
    rooms[idRoom].users.forEach((value, key) => {
        // console.log(key)
        value.send(JSON.stringify({
            type: "newQuestion",
            opening: opening
        }))
    })
}

app.listen(1000)

function readyPlayer(roomId, clientId) {
    for(let i = 0; i < rooms[roomId]["users"].length; i++) {
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

    if (ready >= 50) {
        rooms[roomId]['status'] = "playing"
        newOpening(roomId)
    }
}

async function newOpening(roomId) {
    if (rooms[roomId]['status'] == 'wait') {
        return
    }
    
    const openingId = getPlayingOpemimg(rooms[roomId]['openings'],rooms[roomId]['history'])
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
            opening: opening,
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
        url: "",
        id: anime['animeid'],
        name: anime['nameRU']
    }
}

async function openingIsLoaded(roomId, clientId) {
    console.log(roomId, clientId)

    for(let i = 0; i < rooms[roomId]["users"].length; i++) {
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

            setTimeout(newOpening, 5000, roomId)
        })
    }, 10000)
}