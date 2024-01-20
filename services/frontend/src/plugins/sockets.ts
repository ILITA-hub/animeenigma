
// import type { App } from 'vue'
// import { io } from "socket.io-client";
// import { useUser } from '@/stores/user';

// interface SocketOptions {
//     baseUrl?: string
//     token?: string
// }

// export default {
//     install: (app: App, options: SocketOptions) => {

//         const url = window.location.origin // + options.baseUrl;

//         const socket = io(url, {
//             reconnectionDelayMax: 10000,
//             transports: ['websocket'],
//             // path: '/ws',
//             auth: {
//               token: options.token
//             },
//             // query: {
//             //   "my-key": "my-value"
//             // }
//         });

//         const user = useUser();

//         socket.on("disconnect", (reason) => {
//             if (reason === "io server disconnect") {
//               // the disconnection was initiated by the server, you need to reconnect manually
//               socket.connect();
//             }
//             // else the socket will automatically try to reconnect
//         });

//         socket.on('connect', () => {
//             console.log('connected');
//         });

//         socket.on('connect_error', (err) => {
//             console.log('connect_error', err.message); // prints the message associated with the error
//         });

//         socket.on('error', (err) => {
//             console.log('error', err.message); // prints the message associated with the error
//         });

//         socket.on('message', (message) => {
//             console.log('message', message);
//         });
        

//         socket.on('login', (message) => {
//             user.token = message.token;
//         });



//         app.config.globalProperties.$socket = socket;
        
//     }
// }
