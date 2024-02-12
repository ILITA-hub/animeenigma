
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
import { getUserByID,  getCache, delCache, setCache } from './helper/init.js'
import { play } from './playing/init.js'

const app = express();
const server = http.createServer(app);
const io = new ioServer(server);
const appPort = process.env.PORT;
const id = process.env.ID;

const clients = {}
const room = await getCache(`room${id}`)

if (!room) {
  throw new Error('Room', id, 'not exist');
}

// app.use(require('morgan')('dev')); // logs

io.on('connection', async (socket) => {
  const client = socket;
  // console.log(socket.handshake.query.sessionId)
  console.log(`Client connected: ${client.id}`);
  const sessionID = String(client.handshake.query.sessionId)

  if (!sessionID) {
    client.emit('login', 'unauthorized');
    client.disconnect(true);
    return;
  }

  const userSession = await getCache(`userSession${sessionID}`)

  if (!userSession) {
    client.emit('login', 'unauthorized');
    client.disconnect(true);
    return;
  }

  const user = await getUserByID(userSession.userId);
  client['user'] = user;
  
  clients[client.id] = client

  room.users[client.id] = user
  clients[client.id].lastPongTime = Date.now()

  // await setCache('room' + room['id'], room);

  broadcastMessage('roomUpdate', room)

  let result = await play(room['rangeOpenings'],room.historyAnime)
  broadcastMessage('plays', result)

  client.on('play', async () => {
    let result = await play(room['rangeOpenings'],room.historyAnime)
    broadcastMessage('plays', result)
    // setInterval(playGame, 1000 * 20)
  })

  client.on("disconnect", async () => {
    if (!clients[client.id]) {
      console.warn(`No client to disconnect: ${client.id}`);
      return;
    }

    if (room.users[client.id]) {
      delete room.users[client.id];
      // await setCache('room' + room['id'], room);
      broadcastMessage('roomUpdate', room);
    }
    
    delete clients[client.id];
    console.log(`Client disconnected: ${client.id}`);

  });

  client.on('pong', data => {
    client.lastPongTime = Date.now();
    room.users[client.id].ping = client.lastPongTime - client.lastPingTime;
    // console.log('ping set to', client.id,  room.users[client.id].ping)
    
    
  })
});

function broadcastMessage(event, payload) {
  for (const client in clients) {
    clients[client].emit(event, payload);
  }
}

async function regularUpdate() {
  const roomOld = await getCache(`room${id}`)

  if (roomOld != room) {
    await setCache('room' + room['id'], room);
  }

  for (const clientId in clients) {
    const client = clients[clientId];

    if ((Date.now() - client.lastPongTime) > (1000 * 60)) { // 1 min
      console.log(`Client auto disconnected: ${client.id}`);
      client.disconnect(true);
      continue;
    }
    
    const ping = Date.now();
    client.lastPingTime = ping;
    client.emit('roomUpdate', { ...room, ping: true });
  }


  for (const clientId in room.users) {
    if (!clients[clientId]) {
      delete room.users[clientId];
      console.log(`Room user auto deleted: ${clientId}`);
    }
  }

}



server.listen(appPort, () => {
  console.log(`listening on *:${appPort}`);
  setInterval(regularUpdate, 1000 * 2); // 2 sec
});