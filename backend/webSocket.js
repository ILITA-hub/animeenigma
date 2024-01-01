import WebSocket, { WebSocketServer } from 'ws'
const port = 9000

const ws = new WebSocketServer({port: port})

ws.on('error', console.error);

ws.on("connection", (wsClient) => {
    console.log(wsClient.send('123'))
})

ws.on("listening", () => {
    console.log(`WebSocket запущен на порту ${port}`)
})

export default ws