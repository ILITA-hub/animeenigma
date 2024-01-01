
class Opening {
    id = 0
    name = ""
    path = ""

    constructor(id, name, path) {
        this.id = id
        this.name = name
        this.path = path
    }

    get getId() {
        return this.id
    }

    get getName() {
        return this.name
    }

    get getPath() {
        return this.path
    }

    set setId(id) {
        this.id = id
    }

    set setName(name) {
        this.name = name
    }

    set setPath(path) {
        this.path = path
    }
}