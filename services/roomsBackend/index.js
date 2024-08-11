import express from "express";
import bodyParser from "body-parser";
import { WebSocketServer } from "ws";
import { getCache } from "./helpers/redis.js" 

const app = express()
app.use(bodyParser.json())

const wss = new WebSocketServer({ port: 1234 })
const rooms = {
    "631b7": {
        users: [], // { id: clientId, ready: true/false, token: token, nickName: nickName, score: 1}
        opening: "",
        timeout: null,
        chat: [] // { nickName: nickName, message: message }
    }
}

const clients = new Map()

app.post("/create_room", (req, res) => {
    res.send(req.body)
})

wss.on("connection", (ws, req) => {
    ws.on("error", console.error)

    ws.on("message", data => {
        const messageBody = JSON.parse(data)
        console.log(messageBody)
        
        switch (messageBody["type"]) {
            case "connect":
                console.log(getCache("userSession"+123))
                rooms[messageBody["roomId"]].users.push(
                    {
                        id: messageBody["user"], 
                        ready: false, 
                        token: "token",
                        nickName: "qwe",
                        score: 1
                    }
                )
                console.log(rooms[messageBody["roomId"]])
                break
            case "newQuestion":
                playVideo("631b7", "//youtube.com/embed/j3p6sXq_uUM")
                break
        }
    })

    const clientId = Date.now() // сделать более уникальным
    clients.set(clientId, ws)

    ws.send(JSON.stringify({
        type: "connect",
        clientId: clientId
    }))

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
            }
        }
    }

    console.log(rooms)
}

function playVideo(idRoom, opening) {
    clients.forEach((value, key) => {
        console.log(key)
        value.send(JSON.stringify({
            type: "newQuestion",
            opening: opening
        }))
    })
}

app.listen(1000)