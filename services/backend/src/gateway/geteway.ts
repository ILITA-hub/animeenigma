import { WebSocketGateway, SubscribeMessage, MessageBody } from "@nestjs/websockets";

@WebSocketGateway()
export class MyGateway {
    @SubscribeMessage("newMessage")
    onNewMessage(@MessageBody() body: any) {
        console.log(body)
    }
}