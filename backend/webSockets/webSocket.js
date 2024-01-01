
import WebSocket, { WebSocketServer } from 'ws';
import config from '../../config/index.js';

const ws = new WebSocketServer({port: config.webSocketPort});

ws.on('error', console.error);

ws.on("connection", (wsClient) => {
    console.log({wsClient})
    console.log(wsClient.send('123'))
});

ws.on('message', (messageAsString) => {
    console.log({messageAsString});
});

ws.on("listening", () => {
    console.log(`WebSocket is listening on port ${config.webSocketPort}`);
});

export default ws;
