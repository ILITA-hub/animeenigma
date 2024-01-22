
import type { App } from 'vue'
import { io } from "socket.io-client";
import { useUserStore } from '@/stores/user';

interface SocketOptions {
    baseUrl?: string
    token?: string
}

export default {
    install: (app: App, options: SocketOptions) => {

        const userStore = useUserStore(); // installed on connection

        const url = window.location.origin // + options.baseUrl;

        const socket = io(url, {
            reconnectionDelayMax: 10000,
            transports: ['websocket'],
            // path: '/ws',
            // auth: {
            //   token: options.token
            // },
            query: {
              sessionId: userStore.getSessionId()
            }
        });

        socket.on("disconnect", (reason) => {
            console.log('disconnected');
            if (reason === "io server disconnect") {
                // the disconnection was initiated by the server, you need to reconnect manually
                if (userStore.getSessionId()) {
                    setTimeout(() => {
                        socket.connect();
                    }, 1000);
                } else {
                    const timer = setInterval(() => {
                        const sessionId = userStore.getSessionId();
                        if (sessionId) {
                            socket.io.opts.query = {
                                sessionId
                            }
                            socket.connect();
                            clearInterval(timer);
                        }
                    }, 1000)
                }
                
            }
            // else the socket will automatically try to reconnect
        });

        socket.on('connect', () => {
            console.log('connected');
        });

        socket.on('connect_error', (err) => {
            console.log('connect_error', err.message); // prints the message associated with the error
        });

        socket.on('error', (err) => {
            console.log('error', err.message); // prints the message associated with the error
        });

        socket.on('message', (message) => {
            console.log('message', message);
        });


        socket['$emit'] = (event: string, data: any) => {
            data.sessionId = userStore.getSessionId();
            return new Promise((resolve, reject) => {
                socket.emit(event, data, (response: any) => {
                    if (response?.error) {
                        reject(response.error);
                    } else {
                        resolve(response);
                    }
                });
            });
        }

        console.log('socket installed', socket)

        app.config.globalProperties.$socket = socket;
        
    }
}
