class Room {
    id = 0
    name = ""
    status = ""
    openingId = ""
    users = {}
    updatedAt = 0
    ownerId = 0
    history = []
    range = []

    constructor(id, name, status, openingId, users, updatedAt, ownerId, history, range) {
        this.id = id
        this.name = name
        this.status = status
        this.openingId = openingId
        this.users = users
        this.updatedAt = updatedAt
        this.ownerId = ownerId
        this.history = history
        this.range = range
    }

    get getId() {
        return this.id
    }

    get getName() {
        return this.name
    }

    get getStatus() {
        return this.status
    }

    get getUsers() {
        return this.users
    }

    get getUpdatedAt() {
        return this.updatedAt
    }

    get getOwnerId() {
        return this.ownerId
    }

    set setId(id) {
        this.id = id
    }

    set setName(name) {
        this.name = name
    }

    set setStatus(status) {
        this.status = status
    }

    set setOpeningId(id) {
        this.openingId = id
    }

    set setUsers(users) {
        this.users = users
    }

    set setUpdateAt(updatedAt) {
        this.updatedAt = updatedAt
    }

    set setOwnerId(ownerId) {
        this.ownerId = ownerId
    }

    addUser(user) {
        this.users[user.id] = {
            quantityTrue : 0
        }
    }

    deleteuser(user) {
        this.users[user.id] = undefined
    }
}

const Status = { START: 'start', PLAYING: 'playing', BREAK: 'break' };

export {Room, Status}