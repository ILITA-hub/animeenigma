
if (!process.env.PORT) {
  throw new Error('PORT not defined');
}

if (!process.env.ID) {
  throw new Error('ID room not defined');
}

import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const express = require('express')
import * as http from 'http';
import { Server as ioServer } from "socket.io";
import { getCache, delCache, setCache } from './redis.js'
import { getUserByID } from './userResources.js'
import { getOpAll, getAnime } from './openingResources.js'

const app = express();
const server = http.createServer(app);
const io = new ioServer(server);
const appPort = process.env.PORT;
const id = process.env.ID;

const clients = {}
const room = await getCache(`room${id}`)

io.on('connection', async (socket) => {
  socket.emit('login', 'Привет')
  console.log(socket.handshake.query.sessionId)
  const sessionID = String(socket.handshake.query.sessionId)

  if (!sessionID) {
    socket.emit('login', 'unauthorized');
    socket.disconnect(true);
    return;
  }

  const userSession = await getCache(`userSession${sessionID}`)

  if (!userSession) {
    socket.emit('login', 'unauthorized');
    socket.disconnect(true);
    return;
  }

  const user = await getUserByID(userSession.userId);
  socket['user'] = user[0];

  broadcastMessage('user connected', { userName: user[0].name })

  clients[socket.id] = socket

  socket.on('play', () => {
    setInterval(playGame, 1000 * 20)
  })

  socket.on("disconnect", () => {
    delete clients[socket.id]
    broadcastMessage('roomUpdate', clients)
  });
});

function broadcastMessage(event, payload) {
  for (const client in clients) {
    clients[client].emit(event, payload);
  }
}

async function playGame() {
  let typeEvent = 0
  if (room['rangeOpenings'].length != 1) {
    typeEvent = room['rangeOpenings'][getRandomArbitrary(0, room['rangeOpenings'].length)]
  }
  const op = await getOP(typeEvent)
  broadcastMessage("play", op)
}

function getRandomInt(min, max) {
  min = Math.ceil(min);
  max = Math.floor(max);
  return Math.floor(Math.random() * (max - min) + min);
}

function filterOpHistory(op) {
  return (room['historyAnime'].indexOf(op.id) === -1) // Если не нашёл то true
}

function addOpHistory (op) {
  if (room['historyAnime'].length >= 20) {
    room['historyAnime'].shift()
  }
  room['historyAnime'].push(op.id)
  room['openingId'] = op.id
}

async function getOP(typeEvent) {
  let result = {
    op: "",
    animeName: []
  }
  let ops
  let op
  switch (typeEvent[type]) {
    case "all":
      while (true) {
        ops = getOpAll();
        op = ops[getRandomInt(0, ops.length)]
        if (filterOpHistory(op)) {
          break
        }
      }
      break
    case "collections":
  }
  addOpHistory(op)
  let resultAnime = await getAnime(op)
  result.op = op.id
  result.animeName = [resultAnime.nameAnime[getRandomInt(0,resultAnime.nameAnime.length)], resultAnime.nameAnime[getRandomInt(0,resultAnime.nameAnime.length)], resultAnime.nameAnime[getRandomInt(0,resultAnime.nameAnime.length)], resultAnime.nameOP]
  return result
}

server.listen(appPort, () => {
  console.log(`listening on *:${appPort}`);
});