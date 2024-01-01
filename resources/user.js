class User {
    id = 0
    nickname = ""
    
    constructor(id, nickname) {
        this.id = id
        this.nickname = nickname
    };

    get getId() {
        return this.id
    }

    get getNickname() {
        return this.nickname
    }

    /**
     * @param {string} nickname
     */
    set setNickname(nickname) {
        this.nickname = nickname
    }
}