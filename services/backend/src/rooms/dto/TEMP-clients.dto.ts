
import { Socket } from 'socket.io';

class Clients {
  [key: string]: Socket

  [Symbol.iterator] = function* () {
    let properties = Object.keys(this);
    for (let i of properties) {
        yield this[i];
    }
  }
}

export { Clients }